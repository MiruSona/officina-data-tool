// Package table 은 GameData/<표>.json 을 읽고 쓴다.
//
// 「파일 전체가 JSON 배열 · 행 하나가 한 줄 · 열 차례는 스키마 차례 · 기본값이면 뺀다」는
// 설계 3장의 규칙 넷을 아는 곳이 프로젝트에서 여기 하나뿐이다.
// UI 도 fmt 도 자기가 JSON 을 만들지 않고 이 묶음을 부른다 — 쓰기 규칙이 둘로 갈리면
// UI 로 저장한 파일과 fmt 로 정리한 파일이 서로 다른 diff 를 낸다 (설계 9장).
//
// 검증 규칙은 모른다. 타입이 맞는지·참조가 있는지는 validate 묶음 몫이다.
package table

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/mirusona/officina-data-tool/internal/textfile"
)

// 데이터 폴더에 있지만 표가 아닌 파일들이다.
const (
	SchemaFileName = "schema.json"
	ConfigFileName = ".datatool.json"
)

// Row 는 데이터 파일의 행 하나다. 파일에 적힌 그대로 들고 있는다 —
// 스키마에 없는 열도 안 버린다. 그걸 잡는 것은 validate 몫이라,
// 여기서 조용히 지우면 사람이 오타를 영영 못 본다.
type Row struct {
	// 이 행의 '{' 가 있는 줄 번호(1부터). 오류를 "item.json:3:" 으로 찍는 데 쓴다.
	Line int
	// 파일에 적힌 열 차례. 스키마 차례와 다를 수 있다 (fmt 가 고칠 몫이다).
	Keys []string
	// 열 이름 → 값. 값은 파일에 적힌 바이트 그대로다.
	Values map[string]json.RawMessage
}

// ID 는 id 열의 값이다. 없거나 문자열이 아니면 빈 문자열이다.
func (r *Row) ID() string {
	raw, ok := r.Values["id"]
	if !ok {
		return ""
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return ""
	}
	return id
}

// Table 은 데이터 파일 한 장이다.
type Table struct {
	// 표 이름 = 파일 이름에서 .json 을 뗀 것이다.
	Name string
	Path string
	Rows []*Row
	// 읽은 파일 바이트 그대로. fmt --check 가 「다시 써도 같은가」를 볼 때 쓴다.
	Raw []byte
}

// Load 는 데이터 파일 한 장을 읽는다.
//
// JSON 이 깨졌거나 파일이 없으면 그냥 오류다 (부르는 쪽에서 종료 4).
func Load(path string) (*Table, error) {
	data, err := textfile.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, path)
}

// Parse 는 데이터 파일 내용을 읽는다. path 는 오류에 찍을 이름이다.
func Parse(data []byte, path string) (*Table, error) {
	t := &Table{Name: TableName(path), Path: path, Raw: data}

	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, brokenJSON(path, err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '[' {
		return nil, fmt.Errorf("%s: 파일 전체가 JSON 배열이어야 한다 (설계 3장)", path)
	}

	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, brokenJSON(path, err)
		}
		line := lineOf(data, valueStart(data, dec.InputOffset(), raw))
		row, err := parseRow(raw, line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		t.Rows = append(t.Rows, row)
	}
	if _, err := dec.Token(); err != nil { // 닫는 ']'
		return nil, brokenJSON(path, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%s: 배열 뒤에 딴 것이 더 있다", path)
	}
	return t, nil
}

// TableName 은 파일 경로에서 표 이름을 뽑는다. 파일 이름이 곧 표 이름이다 (설계 3장).
func TableName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".json")
}

func brokenJSON(path string, err error) error {
	return fmt.Errorf("%s: JSON 이 깨졌다: %w", path, err)
}

// parseRow 는 행 하나를 열 차례를 지키며 읽는다.
// encoding/json 의 맵은 차례를 잃어버려서 Token 으로 직접 훑는다.
func parseRow(raw json.RawMessage, line int) (*Row, error) {
	row := &Row{Line: line, Values: map[string]json.RawMessage{}}

	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("행을 못 읽었다: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("행 하나는 {열: 값} 꼴이어야 한다")
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("행을 못 읽었다: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("열 이름이 문자열이 아니다")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, fmt.Errorf("%s 값을 못 읽었다: %w", key, err)
		}
		// 한 행에 같은 열이 두 번 있으면 막는다.
		// encoding/json 은 뒤엣것으로 조용히 덮어써서, 고친 줄이 안 먹는 채로 지나간다.
		if _, dup := row.Values[key]; dup {
			return nil, fmt.Errorf("열 %q 가 한 행에 두 번 있다", key)
		}
		row.Keys = append(row.Keys, key)
		row.Values[key] = value
	}
	return row, nil
}

// valueStart 는 방금 읽은 값이 시작한 바이트 자리를 찾는다.
//
// Decoder 는 값의 끝자리만 알려주므로 길이를 빼서 되짚는다. 들여쓴 JSON 에서도
// 행의 '{' 자리를 정확히 집으려는 것이다 — 줄 번호가 곧 오류 메시지의 값이다.
func valueStart(data []byte, end int64, raw []byte) int64 {
	if start := end - int64(len(raw)); start >= 0 && bytes.Equal(data[start:end], raw) {
		return start
	}
	// Decoder 가 값 뒤 공백까지 삼킨 경우다. 공백을 걷어내고 한 번 더 되짚는다.
	trimmed := end
	for trimmed > 0 && isSpace(data[trimmed-1]) {
		trimmed--
	}
	if start := trimmed - int64(len(raw)); start >= 0 && bytes.Equal(data[start:trimmed], raw) {
		return start
	}
	return end
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// lineOf 는 바이트 자리가 몇 번째 줄인지 센다 (1부터).
func lineOf(data []byte, offset int64) int {
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	return 1 + bytes.Count(data[:offset], []byte("\n"))
}
