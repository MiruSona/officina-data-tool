package table

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mirusona/officina-data-tool/internal/schema"
)

// Format 은 표를 설계 3장의 규칙 넷대로 적는다.
//
//	① 파일 전체가 유효한 JSON 배열 — '[' 와 ']' 만 자기 줄에 선다
//	② 행 하나가 한 줄
//	③ 열 차례는 스키마 차례
//	④ 기본값과 같은 값은 열을 뺀다
//
// 스키마에 없는 열은 안 적는다. 그런 열이 있는 채로 이것을 부르면 값이 사라지므로,
// 부르는 쪽(`fmt`·`serve`)이 먼저 그런 열이 없는지 보고 온다.
func (t *Table) Format(st *schema.Table) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("[\n")
	for i, row := range t.Rows {
		line, err := formatRow(row, st)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", t.Path, row.Line, err)
		}
		b.Write(line)
		if i < len(t.Rows)-1 {
			b.WriteByte(',') // 마지막 행 뒤에는 쉼표가 없다
		}
		b.WriteByte('\n')
	}
	b.WriteString("]\n") // 파일 끝 개행 하나
	return b.Bytes(), nil
}

func formatRow(row *Row, st *schema.Table) ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	first := true
	for _, col := range st.Columns {
		raw, ok := row.Values[col.Name]
		if !ok {
			continue
		}
		value, err := canonical(raw)
		if err != nil {
			return nil, fmt.Errorf("%s 값을 못 적었다: %w", col.Name, err)
		}
		if isDefault(col, value) {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		fmt.Fprintf(&b, "%q:", col.Name) // 열 이름은 ^[a-z][a-z0-9_]*$ 라 그대로 따옴표만 씌운다
		b.Write(value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// isDefault 는 값이 기본값과 같은지 본다. 둘 다 정규형으로 만들어 비교하므로
// 0 과 0.0, [ ] 와 [] 가 같은 것으로 잡힌다.
func isDefault(col *schema.Column, value []byte) bool {
	if col.Default == nil {
		return false
	}
	def, err := canonical(col.Default)
	if err != nil {
		return false
	}
	return bytes.Equal(value, def)
}

// canonical 은 값 하나를 정규형 JSON 한 줄로 만든다.
//
// 숫자는 json.Number 로 받아 적힌 그대로 남기고(0.05 가 0.05 로 남는다),
// 문자열은 다시 적어 가 같은 이스케이프를 한글 그대로로 편다.
// HTML 이스케이프는 끈다 — <, >, & 가 < 로 바뀌면 사람이 못 읽는다.
func canonical(raw json.RawMessage) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// WriteFile 은 tmp 에 쓰고 이름을 바꾼다.
//
// 쓰다 죽어도 반쯤 쓴 데이터 파일이 안 남는다 (설계 9장의 저장 규칙과 같다).
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
