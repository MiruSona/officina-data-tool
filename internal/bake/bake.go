// Package bake 는 스키마와 데이터를 합쳐 gamedata.bytes 한 장으로 굽는다.
//
// 굽는 꼴은 「16바이트 머리 + MessagePack 본문」이다 (설계 7장).
// 머리가 고정 길이라 2차에 암호화를 켜도 게임 코드가 읽는 자리가 안 움직인다.
//
// 검증(validate)은 이 앞에서 이미 돌았다고 본다. 그래도 타입이 안 맞으면
// **조용히 0 을 넣지 않고 오류를 낸다** — 구운 파일이 조용히 틀리는 것이 제일 나쁘다.
package bake

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/mirusona/officina-data-tool/internal/mpack"
	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
)

// 파일 머리 규격 (설계 7장).
const (
	// Magic 은 파일 첫 네 바이트다. 엉뚱한 파일을 읽는 것을 바로 막는다.
	Magic = "OFDT"
	// FormatVersion 은 지금 굽는 형식판이다. C# 쪽 GameDataLoader.FormatVersion 과 같아야 한다.
	FormatVersion = 1
	// HeaderSize 는 머리 길이다. 2차에 암호화가 붙어도 안 바뀐다.
	HeaderSize = 16

	// FlagEncrypted 는 본문이 암호문이라는 표시다 (2차). 1차는 늘 0 이다.
	FlagEncrypted = 0x01
	// FlagCompressed 는 본문이 압축됐다는 표시다 (2차). 1차는 늘 0 이다.
	FlagCompressed = 0x02
)

// MetaKey 는 본문 맵에서 표가 아닌 자리의 키다. 표 이름은 소문자라 절대 안 겹친다.
const MetaKey = "_meta"

// Error 는 굽다 만난 값 하나의 잘못이다.
// 꼴을 검증 오류와 똑같이 맞춘다 — 고칠 자리가 「파일:줄: 표.열」이라는 것이 같아서다.
type Error struct {
	Path    string
	Line    int
	Table   string
	Column  string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s:%d: %s.%s — %s",
		filepath.ToSlash(e.Path), e.Line, e.Table, e.Column, e.Message)
}

// Bake 는 표 전부를 구워 gamedata.bytes 의 바이트를 만든다.
//
// builtAt 을 인자로 받는 이유는 시험 때문이다 — 같은 데이터·같은 시각이면
// 몇 번을 구워도 바이트가 같아야 한다 (T6).
func Bake(sch *schema.File, tables map[string]*table.Table, builtAt time.Time) ([]byte, error) {
	if sch == nil {
		return nil, fmt.Errorf("스키마가 nil 이다")
	}

	root := map[string]any{
		MetaKey: map[string]any{
			"schemaHash": sch.Hash(),
			"builtAt":    builtAt.UTC().Format(time.RFC3339),
		},
	}
	for _, def := range sch.Tables {
		t := tables[def.Name]
		if t == nil {
			return nil, fmt.Errorf("표 %s 의 데이터가 없다", def.Name)
		}
		rows, err := bakeTable(sch, def, t)
		if err != nil {
			return nil, err
		}
		root[def.Name] = rows
	}

	body, err := mpack.Encode(root)
	if err != nil {
		return nil, err
	}
	return append(writeHeader(0, len(body)), body...), nil
}

// bakeTable 은 표 하나를 행 배열의 배열로 만든다. sch 는 enum 목록을 보려고 받는다.
func bakeTable(sch *schema.File, def *schema.Table, t *table.Table) ([]any, error) {
	rows := make([]any, 0, len(t.Rows))
	for _, row := range t.Rows {
		values := make([]any, 0, len(def.Columns))
		for _, col := range def.Columns {
			v, err := bakeCell(sch, def, col, t, row)
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		}
		rows = append(rows, values)
	}
	return rows, nil
}

// bakeCell 은 칸 하나를 굽는다. 값이 빠졌으면 스키마의 기본값으로 채운다.
//
// **기본값 열도 반드시 값을 채운다** — 행은 배열이라 자리를 비울 수가 없고,
// C# 은 [Key(n)] 의 n 번째 자리에 값이 있다고 믿는다 (설계 7장).
func bakeCell(sch *schema.File, def *schema.Table, col *schema.Column, t *table.Table, row *table.Row) (any, error) {
	raw, ok := row.Values[col.Name]
	if !ok || len(raw) == 0 {
		if col.Default == nil {
			return nil, cellError(def, col, t, row, "값이 없는데 기본값도 없다 (필수 열이다)")
		}
		raw = col.Default
	}
	// 판정은 validate·스키마 검사와 같은 schema.Decode 다 — validate 가 OK 면 여기서도 OK 다.
	v, err := schema.Decode(sch, col, raw)
	if err != nil {
		return nil, cellError(def, col, t, row, err.Error())
	}
	return v, nil
}

func cellError(def *schema.Table, col *schema.Column, t *table.Table, row *table.Row, message string) error {
	return &Error{Path: t.Path, Line: row.Line, Table: def.Name, Column: col.Name, Message: message}
}

// writeHeader 는 16바이트 머리를 만든다.
//
// 본문 길이만 리틀엔디안이다 — MessagePack 본문은 빅엔디안이라 헷갈리기 쉬운 자리다.
// C# 쪽(GameDataLoader.ReadBody)도 같은 자리를 리틀엔디안으로 읽는다.
func writeHeader(flags byte, bodyLength int) []byte {
	head := make([]byte, HeaderSize)
	copy(head[0:4], Magic)
	head[4] = FormatVersion
	head[5] = flags
	// [6..7] 예약 · [12..15] 예약 (2차 nonce 자리) — 0 그대로 둔다.
	n := uint32(bodyLength)
	head[8] = byte(n)
	head[9] = byte(n >> 8)
	head[10] = byte(n >> 16)
	head[11] = byte(n >> 24)
	return head
}

// Header 는 구운 파일의 머리다.
type Header struct {
	Version    byte
	Flags      byte
	BodyLength uint32
}

// ReadHeader 는 머리 16바이트를 읽고 검사한다.
//
// 시험이 쓰고, 2차에 복호화가 쓴다. C# 쪽 GameDataLoader.ReadBody 와 같은 검사를 한다 —
// 둘이 어긋나면 Go 시험은 통과하는데 게임에서만 터진다.
func ReadHeader(data []byte) (Header, error) {
	var h Header
	if len(data) < HeaderSize {
		return h, fmt.Errorf("구운 파일이 머리 %d바이트보다 짧다 : %d바이트", HeaderSize, len(data))
	}
	if string(data[0:4]) != Magic {
		return h, fmt.Errorf("구운 파일이 아니다 — 매직이 %s 가 아니다", Magic)
	}
	h.Version = data[4]
	if h.Version != FormatVersion {
		return h, fmt.Errorf("모르는 형식판이다 : %d (이 코드가 아는 것 : %d)", h.Version, FormatVersion)
	}
	h.Flags = data[5]
	if h.Flags&^byte(FlagEncrypted|FlagCompressed) != 0 {
		return h, fmt.Errorf("모르는 플래그가 섰다 : 0x%02x", h.Flags)
	}
	h.BodyLength = uint32(data[8]) | uint32(data[9])<<8 | uint32(data[10])<<16 | uint32(data[11])<<24
	if have := len(data) - HeaderSize; int64(h.BodyLength) != int64(have) {
		return h, fmt.Errorf("본문 길이가 안 맞다 — 머리는 %d바이트라는데 실제는 %d바이트다", h.BodyLength, have)
	}
	return h, nil
}
