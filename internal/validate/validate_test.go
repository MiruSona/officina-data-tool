package validate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
	"github.com/mirusona/officina-data-tool/internal/validate"
)

const dataRoot = "../../Testdata/validate"

// want 은 문제 하나가 어디에 어떤 규칙으로 서야 하는지다.
// 메시지 글자까지 박으면 말을 다듬을 때마다 시험이 깨지므로 자리와 규칙만 본다.
type want struct {
	file   string
	line   int
	column string
	rule   string
}

// caseDir 은 깨끗한 판을 임시 폴더에 깔고 그 위에 깨진 파일만 덮는다.
// 깨진 판이 자기 파일 하나만 갖고 있으면 「무엇을 깨뜨렸나」가 한눈에 보인다.
func caseDir(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	copyInto(t, dir, filepath.Join(dataRoot, "schema.json"))
	for _, table := range []string{"item.json", "monster.json", "drop.json"} {
		copyInto(t, dir, filepath.Join(dataRoot, "ok", table))
	}
	entries, err := os.ReadDir(filepath.Join(dataRoot, name))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		copyInto(t, dir, filepath.Join(dataRoot, name, entry.Name()))
	}
	return dir
}

func copyInto(t *testing.T, dir, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(path)), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCase(t *testing.T, name string) []*validate.Problem {
	t.Helper()
	dir := caseDir(t, name)
	sch, err := schema.Load(filepath.Join(dir, table.SchemaFileName))
	if err != nil {
		t.Fatal(err)
	}
	tables, err := table.LoadAll(dir, sch)
	if err != nil {
		t.Fatal(err)
	}
	return validate.Run(sch, tables)
}

func checkProblems(t *testing.T, got []*validate.Problem, wants []want) {
	t.Helper()
	if len(got) != len(wants) {
		t.Fatalf("문제 수가 다르다: %d 건, 기대 %d 건\n%s", len(got), len(wants), validate.Text(got))
	}
	for i, w := range wants {
		p := got[i]
		have := want{file: filepath.Base(p.File), line: p.Line, column: p.Column, rule: p.Rule}
		if have != w {
			t.Errorf("%d번째가 다르다\n  얻음: %+v (%s)\n  기대: %+v", i, have, p.Message, w)
		}
	}
}

// 규칙마다 깨뜨린 판 하나씩 — 줄 번호와 열까지 맞는지 본다 (설계 11장 T3).
func TestRules(t *testing.T) {
	cases := []struct {
		name  string
		wants []want
	}{
		{"ok", nil},
		{"v2-unknown", []want{
			{"item.json", 2, "atkk", validate.RuleUnknown},
		}},
		{"v2-required", []want{
			{"monster.json", 2, "hp", validate.RuleRequired},
			{"monster.json", 5, "name", validate.RuleRequired},
		}},
		{"v3-type", []want{
			{"item.json", 2, "atk", validate.RuleType},
			{"item.json", 3, "atk", validate.RuleType},
			{"item.json", 4, "usable", validate.RuleType},
			{"item.json", 5, "name", validate.RuleType},
			{"item.json", 5, "atk", validate.RuleType},
		}},
		// null 은 값이 아니다 — encoding/json 이 조용히 넘기는 자리다 (리뷰 A).
		{"v3-null", []want{
			{"item.json", 2, "name", validate.RuleType},
			{"item.json", 3, "usable", validate.RuleType},
			{"item.json", 4, "atk", validate.RuleType},
			{"item.json", 5, "tags", validate.RuleList},
		}},
		// 12.0 은 값으로는 정수지만 굽는 쪽이 안 받는다 — 여기서 막아야 validate OK·export 실패가 없다 (리뷰 D3).
		{"v3-int-form", []want{
			{"item.json", 2, "atk", validate.RuleType},
		}},
		// 빈 문자열 id — 값이 있어 필수 검사에 안 걸리므로 여기서 꼴·중복을 본다 (리뷰 D2).
		{"v7-empty-id", []want{
			{"item.json", 8, "id", validate.RuleIDForm},
			{"item.json", 9, "id", validate.RuleIDForm},
			{"item.json", 9, "id", validate.RuleDupID},
		}},
		// float64 로는 멀쩡해도 float32 로 못 담는 수 (리뷰 F).
		{"v3-float32", []want{
			{"item.json", 2, "price", validate.RuleType},
		}},
		{"v4-range", []want{
			{"drop.json", 2, "rate", validate.RuleRange},
			{"drop.json", 3, "rate", validate.RuleRange},
		}},
		{"v5-enum", []want{
			{"item.json", 2, "grade", validate.RuleEnum},
			{"item.json", 3, "grade", validate.RuleType},
		}},
		{"v6-ref", []want{
			{"drop.json", 2, "item_id", validate.RuleRef},
			{"drop.json", 3, "monster_id", validate.RuleRef},
		}},
		{"v7-dup", []want{
			{"item.json", 8, "id", validate.RuleDupID},
		}},
		{"v7-form", []want{
			{"item.json", 8, "id", validate.RuleIDForm},
			{"item.json", 9, "id", validate.RuleIDForm},
		}},
		{"v9-list", []want{
			{"item.json", 2, "tags", validate.RuleList},
			{"item.json", 5, "tags[1]", validate.RuleType},
		}},
		{"many", []want{
			{"item.json", 8, "id", validate.RuleIDForm},
			{"item.json", 8, "atk", validate.RuleType},
			{"item.json", 8, "grade", validate.RuleEnum},
			{"item.json", 8, "atkk", validate.RuleUnknown},
			{"item.json", 9, "name", validate.RuleRequired},
			{"item.json", 9, "atk", validate.RuleRange},
			{"item.json", 9, "tags[0]", validate.RuleType},
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkProblems(t, runCase(t, c.name), c.wants)
		})
	}
}

// ref 오타는 가장 가까운 id 를 같이 보여준다 (설계 11장 T4).
// 멀리 떨어진 것에는 아무 말도 안 붙인다 — 엉뚱한 것을 자신 있게 권하지 않으려는 것이다.
func TestRefSuggestsNearest(t *testing.T) {
	problems := runCase(t, "v6-ref")
	if !strings.Contains(problems[0].Message, `"sword_iron"`) {
		t.Errorf("오타에 가까운 id 를 안 보여준다: %s", problems[0].Message)
	}
	if strings.Contains(problems[1].Message, "가장 가까운") {
		t.Errorf("먼 값에 엉뚱한 제안을 붙였다: %s", problems[1].Message)
	}
}

// refSchema 는 ref 열 셋(선택·필수·목록)을 가진 작은 스키마다. TestRefEmpty 가 쓴다.
const refSchema = `{ "version": 1, "namespace": "Test.Data", "tables": [
  { "name": "item", "columns": [ { "name": "id", "type": "string" } ] },
  { "name": "box", "columns": [
    { "name": "id",    "type": "string" },
    { "name": "must",  "type": "ref",       "ref": "item" },
    { "name": "maybe", "type": "ref",       "ref": "item", "default": "" },
    { "name": "other", "type": "ref",       "ref": "item", "default": "sword" },
    { "name": "many",  "type": "list<ref>", "ref": "item", "default": [] } ] } ] }`

// ref 칸의 빈 값은 asset 열과 같은 규칙이다 — 선택 열이면 「없음」, 필수 열이면 V2, 목록 안이면 V6.
func TestRefEmpty(t *testing.T) {
	cases := []struct {
		name     string
		row      string
		problems []want
	}{
		{"ok", `{"id":"a","must":"sword","maybe":"sword","many":["sword"]}`, nil},
		{"optional-empty", `{"id":"a","must":"sword","maybe":""}`, nil},
		// default 가 무엇이든 있기만 하면 선택 열이다 — 칸의 "" 는 「없음」으로 통과한다.
		{"optional-empty-with-id-default", `{"id":"a","must":"sword","other":""}`, nil},
		{"required-empty", `{"id":"a","must":""}`, []want{{"box.json", 2, "must", validate.RuleRequired}}},
		{"list-empty", `{"id":"a","must":"sword","many":[""]}`, []want{{"box.json", 2, "many[0]", validate.RuleRef}}},
		{"typo", `{"id":"a","must":"swrod"}`, []want{{"box.json", 2, "must", validate.RuleRef}}},
	}
	sch, err := schema.Parse([]byte(refSchema), "schema.json")
	if err != nil {
		t.Fatal(err)
	}
	item, err := table.Parse([]byte("[\n{\"id\":\"sword\"}\n]\n"), "item.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			box, err := table.Parse([]byte("[\n"+c.row+"\n]\n"), "box.json")
			if err != nil {
				t.Fatal(err)
			}
			checkProblems(t, validate.Run(sch, map[string]*table.Table{"item": item, "box": box}), c.problems)
		})
	}
}

// 오류 한 줄의 꼴은 `파일:줄: 표.열 — 무엇이 잘못됐나` 다 (설계 6장).
func TestProblemLineShape(t *testing.T) {
	problems := runCase(t, "v5-enum")
	line := problems[0].String()
	if !strings.Contains(line, "item.json:2: item.grade — ") {
		t.Errorf("꼴이 다르다: %s", line)
	}
}

// 표 하나에서 쏟아지는 문제는 100건에서 자르고 남은 수를 알린다 (설계 6장).
func TestTooManyProblemsAreCut(t *testing.T) {
	sch, err := schema.Load(filepath.Join(dataRoot, "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]string, 0, 150)
	for i := 0; i < 150; i++ {
		rows = append(rows, fmt.Sprintf(`{"id":"item_%03d","name":"이름","atkk":1}`, i))
	}
	data := []byte("[\n" + strings.Join(rows, ",\n") + "\n]\n")
	item, err := table.Parse(data, "item.json")
	if err != nil {
		t.Fatal(err)
	}

	problems := validate.Run(sch, map[string]*table.Table{"item": item})
	if len(problems) != validate.MaxPerTable+1 {
		t.Fatalf("자른 뒤 문제 수가 %d 다", len(problems))
	}
	last := problems[len(problems)-1]
	if last.Rule != validate.RuleTooMany || !strings.Contains(last.Message, "50건") {
		t.Errorf("남은 수를 안 알린다: %s", last.String())
	}
}
