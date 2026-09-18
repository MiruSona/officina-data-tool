package schema

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mirusona/officina-data-tool/internal/textfile"
)

// 표 이름·열 이름 규칙. 파일 이름이 곧 표 이름이라 소문자 한 낱말로 묶는다.
var reLowerName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// enum 이름은 C# 타입 이름이 되므로 대문자로 시작해도 된다.
var reEnumName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// enum 값은 데이터 파일에 그대로 들어가므로 id 와 같은 꼴로 묶는다.
var reEnumValue = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

var reNamespace = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// 열 칸은 설계 4-3 의 아홉이 전부다. 더 늘리지 않는다.
var columnKeys = []string{"name", "type", "default", "min", "max", "enum", "ref", "loc", "desc"}

var baseTypes = []string{TypeInt, TypeFloat, TypeBool, TypeString, TypeEnum, TypeRef}

// Load 는 스키마 파일을 읽어 검사까지 마친 File 을 준다.
//
// 돌려주는 오류는 두 갈래다. 구조가 틀리면 *Errors (종료 3), 파일을 못 읽거나
// JSON 이 깨졌으면 그 밖의 오류 (종료 4) — 고칠 자리가 다르기 때문이다.
func Load(path string) (*File, error) {
	data, err := textfile.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, path)
}

// Parse 는 스키마 내용을 읽어 검사한다. name 은 오류 앞에 붙일 파일 이름이다.
func Parse(data []byte, name string) (*File, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%s: JSON 이 깨졌다: %w", name, err)
	}

	c := &collector{file: name}
	f := &File{Enums: map[string][]string{}}
	checkKeys(c, "", root, "version", "namespace", "enums", "tables")
	parseVersion(c, root, f)
	parseNamespace(c, root, f)
	parseEnums(c, root, f)
	parseTables(c, root, f)
	checkReferences(c, f)

	if err := c.err(); err != nil {
		return nil, err
	}
	return f, nil
}

func parseVersion(c *collector, root map[string]json.RawMessage, f *File) {
	raw, ok := root["version"]
	if !ok {
		c.add("version", "칸이 없다")
		return
	}
	if err := json.Unmarshal(raw, &f.Version); err != nil {
		c.add("version", "정수가 와야 한다")
		return
	}
	if f.Version < 1 {
		c.add("version", "1 이상이어야 한다")
	}
}

func parseNamespace(c *collector, root map[string]json.RawMessage, f *File) {
	raw, ok := root["namespace"]
	if !ok {
		c.add("namespace", "칸이 없다")
		return
	}
	if err := json.Unmarshal(raw, &f.Namespace); err != nil {
		c.add("namespace", "문자열이 와야 한다")
		return
	}
	if !reNamespace.MatchString(f.Namespace) {
		c.add("namespace", fmt.Sprintf("C# 네임스페이스 꼴이 아니다: %q", f.Namespace))
	}
}

func parseEnums(c *collector, root map[string]json.RawMessage, f *File) {
	raw, ok := root["enums"]
	if !ok {
		return // enum 을 안 쓰는 스키마도 된다
	}
	var enums map[string][]string
	if err := json.Unmarshal(raw, &enums); err != nil {
		c.add("enums", "{이름: [값…]} 꼴이 와야 한다")
		return
	}
	for _, name := range sortedKeys(enums) {
		checkEnum(c, name, enums[name])
		f.Enums[name] = enums[name]
	}
}

func checkEnum(c *collector, name string, values []string) {
	where := "enums." + name
	if !reEnumName.MatchString(name) {
		c.add(where, fmt.Sprintf("이름 꼴이 틀렸다 (%s): %q", reEnumName, name))
	}
	if len(values) == 0 {
		c.add(where, "값이 하나도 없다")
		return
	}
	seen := map[string]bool{}
	for i, v := range values {
		at := fmt.Sprintf("%s[%d]", where, i)
		if !reEnumValue.MatchString(v) {
			c.add(at, fmt.Sprintf("값 꼴이 틀렸다 (%s): %q", reEnumValue, v))
		}
		if seen[v] {
			c.add(at, fmt.Sprintf("값 %q 가 두 번 있다", v))
		}
		seen[v] = true
	}
}

func parseTables(c *collector, root map[string]json.RawMessage, f *File) {
	raw, ok := root["tables"]
	if !ok {
		c.add("tables", "칸이 없다")
		return
	}
	var rawTables []json.RawMessage
	if err := json.Unmarshal(raw, &rawTables); err != nil {
		c.add("tables", "배열이 와야 한다")
		return
	}
	if len(rawTables) == 0 {
		c.add("tables", "표가 하나도 없다")
		return
	}

	seen := map[string]int{}
	for i, one := range rawTables {
		t := parseTable(c, fmt.Sprintf("tables[%d]", i), one)
		if t == nil {
			continue
		}
		if at, dup := seen[t.Name]; dup {
			c.add(fmt.Sprintf("tables[%d].name", i),
				fmt.Sprintf("표 %q 가 tables[%d] 에 이미 있다", t.Name, at))
		}
		seen[t.Name] = i
		f.Tables = append(f.Tables, t)
	}
}

func parseTable(c *collector, where string, raw json.RawMessage) *Table {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		c.add(where, "{name, columns} 꼴이 와야 한다")
		return nil
	}
	checkKeys(c, where, fields, "name", "columns")

	t := &Table{where: where}
	if err := json.Unmarshal(fields["name"], &t.Name); err != nil || t.Name == "" {
		c.add(where+".name", "표 이름이 없다")
		return nil
	}
	if !reLowerName.MatchString(t.Name) {
		c.add(where+".name", fmt.Sprintf("이름 꼴이 틀렸다 (%s): %q", reLowerName, t.Name))
	}

	t.Columns = parseColumns(c, where, fields["columns"])
	if len(t.Columns) == 0 {
		return t
	}
	checkFirstColumn(c, where, t.Columns[0])
	return t
}

// 첫 열은 반드시 id · string 이다. 예외를 안 만든다 (설계 4-3).
//
// **id 에는 꾸밈 칸을 못 붙인다.** id 는 행을 가리키는 열쇠라 늘 값이 있어야 하고
// (default 가 있으면 빈 열쇠가 생긴다), 열쇠에 범위·목록·참조를 걸 자리가 없다.
// 붙여 봐야 조용히 무시되므로 여기서 막는다 — 무시되는 칸은 사람을 속인다.
func checkFirstColumn(c *collector, where string, first *Column) {
	at := where + ".columns[0]"
	if first.Name != "id" {
		c.add(at+".name", fmt.Sprintf("첫 열은 반드시 id 여야 한다: %q", first.Name))
	}
	if first.Type != TypeString {
		c.add(at+".type", fmt.Sprintf("첫 열 id 는 반드시 string 이어야 한다: %q", first.Type))
	}

	banned := []struct {
		key string
		has bool
	}{
		{"default", first.Default != nil},
		{"min", first.Min != nil},
		{"max", first.Max != nil},
		{"loc", first.Loc},
		{"enum", first.Enum != ""},
		{"ref", first.Ref != ""},
	}
	for _, b := range banned {
		if b.has {
			c.add(at+"."+b.key, fmt.Sprintf("첫 열 id 에는 %s 칸을 못 붙인다 (쓸 수 있는 것: name, type, desc)", b.key))
		}
	}
}

func parseColumns(c *collector, where string, raw json.RawMessage) []*Column {
	if len(raw) == 0 {
		c.add(where+".columns", "칸이 없다")
		return nil
	}
	var rawColumns []json.RawMessage
	if err := json.Unmarshal(raw, &rawColumns); err != nil {
		c.add(where+".columns", "배열이 와야 한다")
		return nil
	}
	if len(rawColumns) == 0 {
		c.add(where+".columns", "열이 하나도 없다")
		return nil
	}

	columns := make([]*Column, 0, len(rawColumns))
	seen := map[string]int{}
	for i, one := range rawColumns {
		at := fmt.Sprintf("%s.columns[%d]", where, i)
		col := parseColumn(c, at, one)
		if col == nil {
			continue
		}
		if prev, dup := seen[col.Name]; dup {
			c.add(at+".name", fmt.Sprintf("열 %q 가 columns[%d] 에 이미 있다", col.Name, prev))
		}
		seen[col.Name] = i
		columns = append(columns, col)
	}
	return columns
}

func parseColumn(c *collector, where string, raw json.RawMessage) *Column {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		c.add(where, "{name, type, …} 꼴이 와야 한다")
		return nil
	}
	checkKeys(c, where, fields, columnKeys...)

	col := &Column{where: where}
	if err := json.Unmarshal(fields["name"], &col.Name); err != nil || col.Name == "" {
		c.add(where+".name", "열 이름이 없다")
		return nil
	}
	if !reLowerName.MatchString(col.Name) {
		c.add(where+".name", fmt.Sprintf("이름 꼴이 틀렸다 (%s): %q", reLowerName, col.Name))
	}
	if !parseColumnType(c, where, fields["type"], col) {
		return nil
	}
	parseColumnOptions(c, where, fields, col)
	return col
}

// parseColumnType 은 type 칸을 읽어 Base·IsList 를 채운다. 못 읽으면 false.
func parseColumnType(c *collector, where string, raw json.RawMessage, col *Column) bool {
	if err := json.Unmarshal(raw, &col.Type); err != nil || col.Type == "" {
		c.add(where+".type", "타입 칸이 없다")
		return false
	}
	inner, isList := listElem(col.Type)
	col.IsList = isList
	col.Base = col.Type
	if isList {
		col.Base = inner
	}
	if isList && inner == "" {
		c.add(where+".type", fmt.Sprintf("list 의 원소 타입이 비었다: %q", col.Type))
		return false
	}
	if _, nested := listElem(col.Base); nested {
		c.add(where+".type", fmt.Sprintf("중첩 list 는 안 된다: %q", col.Type))
		return false
	}
	if !contains(baseTypes, col.Base) {
		c.add(where+".type", fmt.Sprintf("모르는 타입이다: %q (쓸 수 있는 것: %s, list<…>)",
			col.Type, strings.Join(baseTypes, ", ")))
		return false
	}
	return true
}

func parseColumnOptions(c *collector, where string, fields map[string]json.RawMessage, col *Column) {
	if raw, ok := fields["default"]; ok {
		col.Default = raw
	}
	col.Min = parseBound(c, where+".min", fields, "min")
	col.Max = parseBound(c, where+".max", fields, "max")
	parseStringOption(c, where+".enum", fields, "enum", &col.Enum)
	parseStringOption(c, where+".ref", fields, "ref", &col.Ref)
	parseStringOption(c, where+".desc", fields, "desc", &col.Desc)
	if raw, ok := fields["loc"]; ok {
		if err := json.Unmarshal(raw, &col.Loc); err != nil {
			c.add(where+".loc", "참거짓이 와야 한다")
		}
	}
}

func parseBound(c *collector, where string, fields map[string]json.RawMessage, key string) *float64 {
	raw, ok := fields[key]
	if !ok {
		return nil
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		c.add(where, "숫자가 와야 한다")
		return nil
	}
	return &v
}

func parseStringOption(c *collector, where string, fields map[string]json.RawMessage, key string, out *string) {
	raw, ok := fields[key]
	if !ok {
		return
	}
	if err := json.Unmarshal(raw, out); err != nil {
		c.add(where, "문자열이 와야 한다")
	}
}

// listElem 은 "list<string>" 에서 "string" 을 떼어낸다.
func listElem(t string) (string, bool) {
	if !strings.HasPrefix(t, "list<") || !strings.HasSuffix(t, ">") {
		return "", false
	}
	return t[len("list<") : len(t)-1], true
}

func checkKeys(c *collector, where string, fields map[string]json.RawMessage, allowed ...string) {
	for _, key := range sortedKeys(fields) {
		if contains(allowed, key) {
			continue
		}
		at := key
		if where != "" {
			at = where + "." + key
		}
		c.add(at, fmt.Sprintf("모르는 칸이다 (쓸 수 있는 것: %s)", strings.Join(allowed, ", ")))
	}
}

func contains(list []string, v string) bool {
	for _, one := range list {
		if one == v {
			return true
		}
	}
	return false
}

// sortedKeys 는 맵 차례가 판마다 달라 오류 차례가 흔들리는 것을 막는다.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
