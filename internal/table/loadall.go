package table

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mirusona/officina-data-tool/internal/schema"
)

// SetError 는 데이터 폴더의 파일 집합이 스키마와 어긋난 것이다 (검증 규칙 V8).
// 이 타입이면 데이터가 틀린 것이므로 종료 2 다 — 스키마를 봐도 소용없다.
type SetError struct {
	Dir string
	// 스키마에 있는데 파일이 없는 표들 ("monster.json").
	Missing []string
	// 파일은 있는데 스키마에 없는 표들 ("items.json").
	Unknown []string
}

func (e *SetError) Error() string {
	lines := []string{}
	for _, name := range e.Missing {
		lines = append(lines, fmt.Sprintf("%s: 스키마에 있는 표인데 파일이 없다",
			filepath.ToSlash(filepath.Join(e.Dir, name))))
	}
	for _, name := range e.Unknown {
		lines = append(lines, fmt.Sprintf("%s: 스키마에 없는 표다 (파일 이름이 곧 표 이름이다)",
			filepath.ToSlash(filepath.Join(e.Dir, name))))
	}
	return strings.Join(lines, "\n")
}

// LoadAll 은 데이터 폴더의 표를 스키마에 적힌 것 전부 읽는다.
//
// 스키마의 표마다 파일이 하나씩 있어야 하고, 스키마에 없는 *.json 이 있으면 막는다.
// schema.json 과 .datatool.json 은 표가 아니라 세지 않는다.
func LoadAll(dir string, sch *schema.File) (map[string]*Table, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	setErr := &SetError{Dir: dir}
	found := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if name == SchemaFileName || name == ConfigFileName {
			continue
		}
		table := strings.TrimSuffix(name, ".json")
		if sch.Table(table) == nil {
			setErr.Unknown = append(setErr.Unknown, name)
			continue
		}
		found[table] = true
	}
	for _, name := range sch.TableNames() {
		if !found[name] {
			setErr.Missing = append(setErr.Missing, name+".json")
		}
	}
	if len(setErr.Missing) > 0 || len(setErr.Unknown) > 0 {
		return nil, setErr
	}

	tables := make(map[string]*Table, len(sch.Tables))
	for _, name := range sch.TableNames() {
		t, err := Load(filepath.Join(dir, name+".json"))
		if err != nil {
			return nil, err
		}
		tables[name] = t
	}
	return tables, nil
}
