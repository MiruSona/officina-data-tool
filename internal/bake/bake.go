// Package bake 는 스키마와 데이터를 합쳐 gamedata.bytes 한 장으로 굽는다.
//
// 굽는 꼴은 「16바이트 머리 + MessagePack 본문」이다 (설계 7장).
// 머리가 고정 길이라 2차에 암호화를 켜도 게임 코드가 읽는 자리가 안 움직인다.
//
// 검증(validate)은 이 앞에서 이미 돌았다고 본다. 그래도 타입이 안 맞으면
// **조용히 0 을 넣지 않고 오류를 낸다** — 구운 파일이 조용히 틀리는 것이 제일 나쁘다.
package bake

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
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
		rows, err := bakeTable(def, t)
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

// bakeTable 은 표 하나를 행 배열의 배열로 만든다.
func bakeTable(def *schema.Table, t *table.Table) ([]any, error) {
	rows := make([]any, 0, len(t.Rows))
	for _, row := range t.Rows {
		values := make([]any, 0, len(def.Columns))
		for _, col := range def.Columns {
			v, err := bakeCell(def, col, t, row)
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
func bakeCell(def *schema.Table, col *schema.Column, t *table.Table, row *table.Row) (any, error) {
	raw, ok := row.Values[col.Name]
	if !ok || len(raw) == 0 {
		if col.Default == nil {
			return nil, cellError(def, col, t, row, "값이 없는데 기본값도 없다 (필수 열이다)")
		}
		raw = col.Default
	}
	v, err := convert(col, raw)
	if err != nil {
		return nil, cellError(def, col, t, row, err.Error())
	}
	return v, nil
}

func cellError(def *schema.Table, col *schema.Column, t *table.Table, row *table.Row, message string) error {
	return &Error{Path: t.Path, Line: row.Line, Table: def.Name, Column: col.Name, Message: message}
}

// convert 는 JSON 값 하나를 mpack 이 아는 Go 값으로 바꾼다.
// list 면 배열을 풀어 원소마다 Base 타입으로 바꾼다 — 중첩 list 는 스키마가 이미 막았다.
func convert(col *schema.Column, raw json.RawMessage) (any, error) {
	if !col.IsList {
		return convertScalar(col, raw)
	}

	var items []json.RawMessage
	if isNull(raw) || json.Unmarshal(raw, &items) != nil {
		return nil, fmt.Errorf("배열이 와야 하는데 %s 다", shorten(raw))
	}
	out := make([]any, 0, len(items))
	for i, item := range items {
		v, err := convertScalar(col, item)
		if err != nil {
			return nil, fmt.Errorf("%d번째 원소가 틀렸다 : %w", i, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// convertScalar 는 값 하나를 Base 타입으로 바꾼다.
// enum·ref 는 굽는 자리에서는 그냥 문자열이다 — 이름 문자열로 넣는다 (설계 7장).
func convertScalar(col *schema.Column, raw json.RawMessage) (any, error) {
	// null 을 맨 앞에서 막는다 (이중 방어).
	// encoding/json 은 null 을 string·bool 에 넣어도 오류를 안 내고 지나가므로,
	// 그냥 두면 `{"name":null}` 이 조용히 ""·false 로 구워진다. 검증이 이미 막지만 여기서도 막는다.
	if isNull(raw) {
		return nil, fmt.Errorf("%s 이 와야 하는데 null 이다 (null 은 값이 아니다)", col.Base)
	}
	switch col.Base {
	case schema.TypeInt:
		return toInt(raw)
	case schema.TypeFloat:
		return toFloat32(raw)
	case schema.TypeBool:
		return toBool(raw)
	case schema.TypeString, schema.TypeEnum, schema.TypeRef:
		return toString(col, raw)
	}
	return nil, fmt.Errorf("모르는 타입 %q 다", col.Type)
}

// toInt 는 정수만 받는다. 12.5 를 12 로 깎지 않는다 — 조용히 값을 바꾸는 쪽이 더 나쁘다.
func toInt(raw json.RawMessage) (any, error) {
	num, err := number(raw)
	if err != nil {
		return nil, fmt.Errorf("int 가 와야 하는데 %s 다", shorten(raw))
	}
	v, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("int 가 와야 하는데 %s 다", shorten(raw))
	}
	return v, nil
}

// toFloat32 는 float32 로 낮춘다. 굽는 값은 전부 단정밀도다 (설계 7장).
//
// 범위를 넘은 수(1e39 같은)는 **수가 아닌 것과 갈라서** 알린다 — 고칠 방법이 다르다.
// 「float 가 와야 한다」는 오타를 고치라는 말이고, 「float32 로 담을 수 없다」는 값을 줄이라는 말이다.
func toFloat32(raw json.RawMessage) (any, error) {
	num, err := number(raw)
	if err != nil {
		return nil, fmt.Errorf("float 가 와야 하는데 %s 다", shorten(raw))
	}
	v, err := strconv.ParseFloat(num, 32)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return nil, fmt.Errorf("float32 로 담을 수 없다 : %s (C# float 은 32비트다)", shorten(raw))
		}
		return nil, fmt.Errorf("float 가 와야 하는데 %s 다", shorten(raw))
	}
	return float32(v), nil
}

// isNull 은 값이 JSON null 인지 본다. 검증 쪽(validate)과 같은 판단이다.
func isNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

// number 는 JSON 값이 **진짜 수**일 때만 그 글자를 준다.
//
// 따옴표를 먼저 막는 이유 : encoding/json 은 "12" 같은 문자열도 json.Number 로 받아 준다.
// 그대로 두면 `"atk":"12"` 가 조용히 12 로 구워진다 (검증 V3 이 잡을 몫인데 여기서도 막는다).
func number(raw json.RawMessage) (string, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s[0] == '"' {
		return "", fmt.Errorf("수가 아니다")
	}
	var num json.Number
	if err := json.Unmarshal(raw, &num); err != nil {
		return "", err
	}
	return num.String(), nil
}

func toBool(raw json.RawMessage) (any, error) {
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("bool 이 와야 하는데 %s 다", shorten(raw))
	}
	return v, nil
}

func toString(col *schema.Column, raw json.RawMessage) (any, error) {
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%s 이 와야 하는데 %s 다", col.Base, shorten(raw))
	}
	return v, nil
}

// shorten 은 오류에 찍을 값을 한 줄로 줄인다. 긴 배열이 오류 메시지를 덮지 않게.
func shorten(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 40 {
		s = s[:40] + "…"
	}
	return s
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
