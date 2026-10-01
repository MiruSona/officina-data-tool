package validate

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mirusona/officina-data-tool/internal/assetindex"
	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
)

// MaxPerTable 은 표 하나에서 내는 문제의 최대 개수다 (설계 6장).
// 스키마를 바꾼 직후 수천 줄이 쏟아지는 것을 막는다. 넘으면 「…외 N건」으로 자른다.
const MaxPerTable = 100

// 규칙 하나가 함수 하나다.
//
//	V2 스키마에 없는 열   checkUnknownColumns
//	V2 필수 열이 비었다   checkColumn (값이 없을 때)
//	V3 타입              checkScalar
//	V4 min·max           checkBounds
//	V5 enum              checkEnum
//	V6 ref               checkRef
//	V7 id 중복·꼴         checkID
//	V9 list<T> 원소       checkList
//	V10 asset            checkAsset (asset.go)
//
// V1(JSON 이 깨졌나)과 V8(파일 이름 = 표 이름)은 table 묶음이 읽을 때 이미 막는다.

// idForm 은 id 로 쓸 수 있는 꼴이다 (설계 6장 V7).
var idForm = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// intForm 은 int 열에 적을 수 있는 글자 꼴이다. 굽는 쪽(bake.toInt)이 받는 꼴과 같다 —
// `12` 는 되고 `12.0`·`1e3` 은 안 된다.
var intForm = regexp.MustCompile(`^-?[0-9]+$`)

// Run 은 데이터 전부를 검사해 문제를 모아 준다. 문제가 없으면 빈 조각이다.
//
// 첫 오류에서 안 멈춘다 — 한 번에 다 고치게 하려는 것이다.
// tables 는 표 이름 → 읽어 온 표다 (table.LoadAll 이 주는 것 그대로).
func Run(sch *schema.File, tables map[string]*table.Table) []*Problem {
	problems, _ := RunWithIndex(sch, tables, nil)
	return problems
}

// RunWithIndex 는 Run 에 V10 을 더한다. assets 가 nil 이면 V10 을 건너뛴다.
// 경고는 오류와 따로 준다 — 경고만 있으면 통과다.
func RunWithIndex(sch *schema.File, tables map[string]*table.Table, assets *assetindex.Index) ([]*Problem, []*Problem) {
	return RunWith(sch, tables, Options{Assets: assets})
}

// Options 는 RunWith 의 손잡이다. AssetsAsWarnings 면 V10 오류를 처음부터 경고로 모은다 (serve 저장 때).
type Options struct {
	Assets           *assetindex.Index
	AssetsAsWarnings bool
}

// RunWith 는 RunWithIndex 에 손잡이를 더한 것이다. 오류와 경고는 따로 자른다.
func RunWith(sch *schema.File, tables map[string]*table.Table, opt Options) ([]*Problem, []*Problem) {
	assets := opt.Assets
	index := buildIDIndex(sch, tables)

	problems := []*Problem{}
	warnings := []*Problem{}
	for _, name := range sch.TableNames() {
		t := tables[name]
		if t == nil {
			continue // 파일이 없는 것은 V8 이 이미 막았다
		}
		c := &checker{sch: sch, st: sch.Table(name), t: t, ids: index, assets: assets, assetWarn: opt.AssetsAsWarnings}
		c.checkTable()
		order := columnOrder(c.st)
		sortProblems(c.list, order)
		sortProblems(c.warns, order)
		problems = append(problems, cut(c.list, t)...)
		warnings = append(warnings, cut(c.warns, t)...)
	}
	return problems, warnings
}

// UnknownColumns 는 「스키마에 없는 열」(V2) 하나만 골라 본다.
//
// `fmt` 가 쓰기 전에 부른다 — Format 은 스키마에 없는 열을 안 적으므로, 오타 난 열이 있는 채로
// 다시 쓰면 그 값이 파일에서 영영 사라진다. 나머지 규칙은 안 돌린다 (fmt 는 검증 명령이 아니다).
func UnknownColumns(sch *schema.File, tables map[string]*table.Table) []*Problem {
	problems := []*Problem{}
	for _, name := range sch.TableNames() {
		t := tables[name]
		if t == nil {
			continue
		}
		c := &checker{sch: sch, st: sch.Table(name), t: t}
		for _, row := range t.Rows {
			c.checkUnknownColumns(row)
		}
		sortProblems(c.list, columnOrder(c.st))
		problems = append(problems, cut(c.list, t)...)
	}
	return problems
}

// checker 는 표 한 장을 보는 동안의 상태다.
type checker struct {
	sch *schema.File
	st  *schema.Table
	t   *table.Table
	// 표 이름 → id → 그 id 가 처음 나온 줄. ref 가 가리키는 곳을 찾는 데 쓴다.
	ids  map[string]map[string]int
	list []*Problem
	// V10 색인. nil 이면 V10 을 건너뛴다.
	assets     *assetindex.Index
	assetWarn  bool // V10 오류를 경고로 모은다
	addressSet map[string]int
	warns      []*Problem
}

// buildIDIndex 는 표마다 id 목록을 미리 모은다.
// 표를 건너 참조하므로 한 표를 보기 전에 모든 표의 id 를 알고 있어야 한다.
func buildIDIndex(sch *schema.File, tables map[string]*table.Table) map[string]map[string]int {
	index := map[string]map[string]int{}
	for _, name := range sch.TableNames() {
		t := tables[name]
		if t == nil {
			continue
		}
		lines := map[string]int{}
		for _, row := range t.Rows {
			if id := row.ID(); id != "" {
				if _, seen := lines[id]; !seen {
					lines[id] = row.Line
				}
			}
		}
		index[name] = lines
	}
	return index
}

// columnOrder 는 열 이름 → 스키마 차례다. 정렬에만 쓴다.
func columnOrder(st *schema.Table) map[string]int {
	order := map[string]int{}
	for i, col := range st.Columns {
		order[col.Name] = i
	}
	return order
}

// cut 은 표 하나의 문제가 너무 많으면 잘라내고 남은 수를 알린다.
func cut(problems []*Problem, t *table.Table) []*Problem {
	if len(problems) <= MaxPerTable {
		return problems
	}
	rest := len(problems) - MaxPerTable
	kept := problems[:MaxPerTable:MaxPerTable]
	return append(kept, &Problem{
		File:    filepath.ToSlash(t.Path),
		Table:   t.Name,
		Rule:    RuleTooMany,
		Message: fmt.Sprintf("…외 %d건 (한 표에 %d건까지만 낸다)", rest, MaxPerTable),
	})
}

func (c *checker) checkTable() {
	seen := map[string]int{}
	for _, row := range c.t.Rows {
		c.checkID(row, seen)
		c.checkUnknownColumns(row)
		c.checkColumns(row)
	}
}

// add 는 문제 하나를 모은다. 자리(파일·줄·표·행)는 늘 같은 데서 채운다.
func (c *checker) add(row *table.Row, column, rule, message string) {
	c.list = append(c.list, c.problem(row, column, rule, message))
}

// warn 은 경고 하나를 모은다. 자리는 add 와 같다.
func (c *checker) warn(row *table.Row, column, rule, message string) {
	c.warns = append(c.warns, c.problem(row, column, rule, message))
}

func (c *checker) problem(row *table.Row, column, rule, message string) *Problem {
	return &Problem{
		File:    filepath.ToSlash(c.t.Path),
		Line:    row.Line,
		Table:   c.t.Name,
		Row:     row.ID(),
		Column:  column,
		Rule:    rule,
		Message: message,
	}
}

// checkID 는 id 가 유일한지·꼴이 맞는지 본다 (V7).
// id 가 없거나 문자열이 아닌 것은 checkColumns 가 알리므로 여기서 또 말하지 않는다.
//
// 다만 **빈 문자열 id 는 여기서 잡는다.** 값이 있으니 필수 검사(V2)에 안 걸리는데
// 여기서도 넘기면 빈 id 가 여럿이어도 중복으로 안 세어, 구운 파일에서 id 가 겹친다.
func (c *checker) checkID(row *table.Row, seen map[string]int) {
	id := row.ID()
	if id == "" && !hasEmptyID(row) {
		return
	}
	if !idForm.MatchString(id) {
		c.add(row, "id", RuleIDForm,
			fmt.Sprintf("id 꼴이 아니다: %q (소문자로 시작하고 소문자·숫자·밑줄만)", id))
	}
	if line, dup := seen[id]; dup {
		c.add(row, "id", RuleDupID, fmt.Sprintf("%q 가 이미 %d번째 줄에 있다", id, line))
		return
	}
	seen[id] = row.Line
}

// hasEmptyID 는 id 칸이 **빈 문자열로 적혀 있는지** 본다. 칸이 없거나 문자열이 아니면 거짓이다.
func hasEmptyID(row *table.Row) bool {
	raw, ok := row.Values["id"]
	if !ok {
		return false
	}
	v, ok := asString(raw)
	return ok && v == ""
}

// checkUnknownColumns 는 스키마에 없는 열을 잡는다 (V2).
func (c *checker) checkUnknownColumns(row *table.Row) {
	for _, key := range row.Keys {
		if c.st.Column(key) == nil {
			c.add(row, key, RuleUnknown,
				fmt.Sprintf("스키마에 없는 열이다 (있는 것: %s)", strings.Join(columnNames(c.st), ", ")))
		}
	}
}

// checkColumns 는 스키마의 열을 하나씩 본다. 값이 없으면 필수인지만 본다 (V2).
func (c *checker) checkColumns(row *table.Row) {
	for _, col := range c.st.Columns {
		raw, ok := row.Values[col.Name]
		if !ok {
			if col.Required() {
				c.add(row, col.Name, RuleRequired, "값이 없다 (기본값이 없는 필수 열이다)")
			}
			continue
		}
		c.checkValue(row, col, col.Name, raw)
	}
}

// checkValue 는 값 하나를 본다. list 면 배열을 풀어 원소마다 같은 규칙을 돌린다 (V9).
func (c *checker) checkValue(row *table.Row, col *schema.Column, where string, raw json.RawMessage) {
	if !col.IsList {
		c.checkScalar(row, col, where, raw)
		return
	}
	c.checkList(row, col, where, raw)
}

// checkList 는 list<T> 열을 본다 (V9).
func (c *checker) checkList(row *table.Row, col *schema.Column, where string, raw json.RawMessage) {
	// null 은 빈 배열이 아니다. encoding/json 이 null 을 조각에 넣어도 오류를 안 내므로 먼저 막는다.
	var items []json.RawMessage
	if isNull(raw) || json.Unmarshal(raw, &items) != nil {
		c.add(row, where, RuleList,
			fmt.Sprintf("%s 열인데 배열이 아니다: %s", col.Type, describe(raw)))
		return
	}
	for i, item := range items {
		c.checkScalar(row, col, fmt.Sprintf("%s[%d]", where, i), item)
	}
}

// checkScalar 는 원소 하나가 열의 타입과 맞는지 본다 (V3).
func (c *checker) checkScalar(row *table.Row, col *schema.Column, where string, raw json.RawMessage) {
	switch col.Base {
	case schema.TypeInt:
		c.checkInt(row, col, where, raw)
	case schema.TypeFloat:
		c.checkFloat(row, col, where, raw)
	case schema.TypeBool:
		if !isBool(raw) {
			c.addType(row, col, where, raw)
		}
	case schema.TypeString:
		if _, ok := asString(raw); !ok {
			c.addType(row, col, where, raw)
		}
	case schema.TypeEnum:
		c.checkEnum(row, col, where, raw)
	case schema.TypeRef:
		c.checkRef(row, col, where, raw)
	case schema.TypeAsset:
		c.checkAsset(row, col, where, raw)
	}
}

func (c *checker) addType(row *table.Row, col *schema.Column, where string, raw json.RawMessage) {
	c.add(row, where, RuleType,
		fmt.Sprintf("%s%s 와야 하는데 %s 다", col.Base, particle(col.Base), describe(raw)))
}

// particle 은 타입 이름 뒤에 붙는 조사다. 「string 가 와야 한다」가 눈에 걸려서 둔다.
func particle(base string) string {
	switch base {
	case schema.TypeInt, schema.TypeFloat, schema.TypeRef:
		return " 가"
	}
	return " 이"
}

// checkInt 는 정수인지 본다. C# 쪽이 int(32비트)라 그 범위도 같이 본다 (설계 4-3 의 타입 표).
func (c *checker) checkInt(row *table.Row, col *schema.Column, where string, raw json.RawMessage) {
	v, ok := asNumber(raw)
	if !ok {
		c.addType(row, col, where, raw)
		return
	}
	if v != math.Trunc(v) {
		c.add(row, where, RuleType, fmt.Sprintf("int 열인데 소수다: %s", string(raw)))
		return
	}
	// 12.0 은 값으로는 정수지만 **굽는 쪽(bake)은 정수 글자만 받는다.**
	// 여기서 안 막으면 validate 는 OK 인데 export 가 터진다.
	if !intForm.MatchString(strings.TrimSpace(string(raw))) {
		c.add(row, where, RuleType,
			fmt.Sprintf("int 열인데 소수 꼴로 적혔다: %s (소수점 없이 적는다)", string(raw)))
		return
	}
	if v < math.MinInt32 || v > math.MaxInt32 {
		c.add(row, where, RuleType,
			fmt.Sprintf("int 가 담을 수 있는 범위를 넘었다: %s (C# int 는 32비트다)", string(raw)))
		return
	}
	c.checkBounds(row, col, where, v)
}

// checkFloat 는 실수인지 본다. 굽는 값이 float32 라 그 범위도 같이 본다 (설계 7장).
//
// float64 로는 멀쩡한 1e39 가 float32 에서는 무한대가 된다. 굽는 자리에서 터지기 전에 여기서 알린다.
func (c *checker) checkFloat(row *table.Row, col *schema.Column, where string, raw json.RawMessage) {
	v, ok := asNumber(raw)
	if !ok {
		c.addType(row, col, where, raw)
		return
	}
	if math.IsNaN(v) || math.IsInf(v, 0) || v < -math.MaxFloat32 || v > math.MaxFloat32 {
		c.add(row, where, RuleType,
			fmt.Sprintf("float 가 담을 수 있는 범위를 넘었다: %s (구울 때 C# float 은 32비트다)", string(raw)))
		return
	}
	c.checkBounds(row, col, where, v)
}

// checkBounds 는 min·max 안인지 본다 (V4). list 면 원소마다 돈다.
func (c *checker) checkBounds(row *table.Row, col *schema.Column, where string, v float64) {
	if col.Min != nil && v < *col.Min {
		c.add(row, where, RuleRange, fmt.Sprintf("min %g 보다 작다: %g", *col.Min, v))
	}
	if col.Max != nil && v > *col.Max {
		c.add(row, where, RuleRange, fmt.Sprintf("max %g 보다 크다: %g", *col.Max, v))
	}
}

// checkEnum 은 값이 enum 목록 안인지 본다 (V5).
func (c *checker) checkEnum(row *table.Row, col *schema.Column, where string, raw json.RawMessage) {
	v, ok := asString(raw)
	if !ok {
		c.addType(row, col, where, raw)
		return
	}
	values, found := c.sch.EnumValues(col.Enum)
	if !found {
		return // 없는 enum 은 스키마 검사가 이미 막았다 (종료 3)
	}
	for _, one := range values {
		if one == v {
			return
		}
	}
	c.add(row, where, RuleEnum, fmt.Sprintf("enum %s 에 없는 값이다: %q (있는 것: %s)",
		col.Enum, v, strings.Join(values, ", ")))
}

// checkRef 는 가리키는 id 가 정말 있는지 본다 (V6).
// 없으면 가장 가까운 id 를 같이 보여준다 — 대부분 오타다 (설계 6장).
func (c *checker) checkRef(row *table.Row, col *schema.Column, where string, raw json.RawMessage) {
	v, ok := asString(raw)
	if !ok {
		c.addType(row, col, where, raw)
		return
	}
	if _, exists := c.ids[col.Ref][v]; exists {
		return
	}
	message := fmt.Sprintf("%s 에 %q 가 없다", col.Ref, v)
	if near := nearest(v, c.ids[col.Ref]); near != "" {
		message += fmt.Sprintf(" (가장 가까운 것: %q)", near)
	}
	c.add(row, where, RuleRef, message)
}

func columnNames(st *schema.Table) []string {
	names := make([]string, 0, len(st.Columns))
	for _, col := range st.Columns {
		names = append(names, col.Name)
	}
	return names
}
