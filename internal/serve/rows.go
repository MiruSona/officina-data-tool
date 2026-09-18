package serve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
)

// readRows 는 PUT 본문에서 행 배열을 꺼낸다.
// 본문은 행 배열 그대로거나 {"rows": [...]} 둘 다 받는다.
func readRows(r *http.Request) ([]json.RawMessage, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("본문을 못 읽었다: %w", err)
	}
	// 깨진 바이트를 그대로 파일에 옮겨 적지 않는다. 데이터 JSON 은 UTF-8 이다.
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("본문이 UTF-8 이 아니다")
	}
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("본문이 비었다 (행 배열을 보낸다)")
	}
	if trimmed[0] == '[' {
		var rows []json.RawMessage
		if err := json.Unmarshal(trimmed, &rows); err != nil {
			return nil, fmt.Errorf("행 배열이 아니다: %w", err)
		}
		return rows, nil
	}
	var body struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(trimmed, &body); err != nil {
		return nil, fmt.Errorf("본문이 JSON 이 아니다: %w", err)
	}
	if body.Rows == nil {
		return nil, fmt.Errorf("rows 칸이 없다")
	}
	return body.Rows, nil
}

// parseRows 는 받은 행들을 데이터 파일 꼴로 이어 붙여 table.Parse 에 맡긴다.
//
// 행을 직접 Row 구조체로 만들지 않는 것은, 열 차례·중복 열·줄 번호를 읽는 규칙이
// table 묶음 하나에만 있어야 하기 때문이다 (설계 9장). 줄 번호도 저절로 맞는다 —
// '[' 가 1번 줄이니 i번째 행이 i+2번 줄이다.
func parseRows(rows []json.RawMessage, path string) (*table.Table, error) {
	var b bytes.Buffer
	b.WriteString("[\n")
	for i, raw := range rows {
		var one bytes.Buffer
		if err := json.Compact(&one, raw); err != nil {
			return nil, fmt.Errorf("%d번째 행이 JSON 이 아니다: %w", i+1, err)
		}
		b.Write(one.Bytes())
		if i < len(rows)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("]\n")
	return table.Parse(b.Bytes(), path)
}

// rowsJSON 은 UI 에 줄 행 배열을 만든다.
//
// 기본값이라 파일에서 빠진 열은 여기서 채운다 — 빈 칸으로 보이면 사람이 값을
// 지운 것인지 원래 없던 것인지 알 수 없다. 저장할 때 Format 이 도로 뺀다.
// 스키마에 없는 열도 그대로 실어 보낸다. 조용히 지우면 오타를 영영 못 본다.
func rowsJSON(t *table.Table, st *schema.Table) []map[string]json.RawMessage {
	out := make([]map[string]json.RawMessage, 0, len(t.Rows))
	for _, row := range t.Rows {
		one := map[string]json.RawMessage{}
		for key, value := range row.Values {
			one[key] = value
		}
		for _, col := range st.Columns {
			if _, ok := one[col.Name]; !ok && col.Default != nil {
				one[col.Name] = col.Default
			}
		}
		out = append(out, one)
	}
	return out
}

// schemaJSON 은 스키마를 UI 가 쓰기 좋은 꼴로 옮긴다.
// 구조체에 json 태그를 다는 대신 여기서 옮기는 것은, schema 묶음이
// 웹 API 의 칸 이름을 모르게 두기 위해서다 (설계 2장의 경계).
func schemaJSON(f *schema.File) map[string]any {
	tables := []map[string]any{}
	for _, t := range f.Tables {
		tables = append(tables, map[string]any{
			"name":    t.Name,
			"columns": columnsJSON(t),
		})
	}
	return map[string]any{
		"ok":        true,
		"version":   f.Version,
		"namespace": f.Namespace,
		"enums":     f.Enums,
		"tables":    tables,
	}
}

func columnsJSON(t *schema.Table) []map[string]any {
	cols := []map[string]any{}
	for _, c := range t.Columns {
		one := map[string]any{
			"name":     c.Name,
			"type":     c.Type,
			"base":     c.Base,
			"isList":   c.IsList,
			"required": c.Required(),
			"desc":     c.Desc,
		}
		if c.Default != nil {
			one["default"] = c.Default
		}
		if c.Min != nil {
			one["min"] = *c.Min
		}
		if c.Max != nil {
			one["max"] = *c.Max
		}
		if c.Enum != "" {
			one["enum"] = c.Enum
		}
		if c.Ref != "" {
			one["ref"] = c.Ref
		}
		cols = append(cols, one)
	}
	return cols
}
