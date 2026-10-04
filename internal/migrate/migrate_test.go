package migrate

import (
	"strings"
	"testing"

	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
	"github.com/mirusona/officina-data-tool/internal/validate"
)

const oldSchema = `{"version":1,"namespace":"G","enums":{"Grade":["common","rare","epic"]},
 "tables":[{"name":"item","columns":[
  {"name":"id","type":"string"},
  {"name":"atk","type":"int","default":0},
  {"name":"grade","type":"enum","enum":"Grade","default":"rare"},
  {"name":"tags","type":"list<enum>","enum":"Grade","default":[]},
  {"name":"note","type":"string","default":""}]}]}`

const itemRows = `[
{"id":"a","atk":3,"grade":"epic","tags":["rare","epic"],"note":"x"},
{"id":"b","grade":"common"},
{"id":"c"}
]
`

type fixture struct {
	old    *schema.File
	tables map[string]*table.Table
}

func load(t *testing.T) fixture {
	t.Helper()
	old, err := schema.Parse([]byte(oldSchema), "schema.json")
	if err != nil {
		t.Fatal(err)
	}
	item, err := table.Parse([]byte(itemRows), "item.json")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{old: old, tables: map[string]*table.Table{"item": item}}
}

// run 은 ops 를 세우고 → default 를 고치고 → 새 스키마를 읽고 → 행을 옮긴다. 서버와 같은 차례다.
func run(t *testing.T, fx fixture, newSchema string, ops ...Op) (*Result, []*validate.Problem) {
	t.Helper()
	p, problems := New(fx.old, ops)
	if len(problems) > 0 {
		return nil, problems
	}
	raw, problems := p.PatchDefaults([]byte(newSchema))
	if len(problems) > 0 {
		return nil, problems
	}
	sch, err := schema.Parse(raw, "schema.json")
	if err != nil {
		t.Fatalf("새 스키마를 못 읽었다: %v\n%s", err, raw)
	}
	return p.Apply(sch, fx.tables)
}

func formatted(t *testing.T, r *Result, _ string) string {
	t.Helper()
	out, err := r.Tables["item"].Format(r.Schema.Table("item"))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func mustRun(t *testing.T, fx fixture, newSchema string, ops ...Op) *Result {
	t.Helper()
	r, problems := run(t, fx, newSchema, ops...)
	if len(problems) > 0 {
		t.Fatalf("문제가 났다: %s", validate.Text(problems))
	}
	return r
}

func TestRenameColumnMovesKeys(t *testing.T) {
	fx := load(t)
	next := strings.Replace(oldSchema, `"name":"atk"`, `"name":"attack"`, 1)
	r := mustRun(t, fx, next, Op{Op: OpRenameColumn, Table: "item", From: "atk", To: "attack"})
	got := formatted(t, r, next)
	if !strings.Contains(got, `{"id":"a","attack":3,`) {
		t.Fatalf("열 이름이 안 바뀌었다:\n%s", got)
	}
	if r.Changes["item"].RowsChanged != 1 {
		t.Fatalf("바뀐 행이 1 이어야 한다: %+v", r.Changes["item"])
	}
	// 원본 표는 안 건드린다 (plan 은 디스크도 메모리도 안 바꾼다).
	if _, ok := fx.tables["item"].Rows[0].Values["atk"]; !ok {
		t.Fatal("원본 표를 고쳤다")
	}
}

func TestColumnGoneWithoutOpIsRefused(t *testing.T) {
	fx := load(t)
	next := strings.Replace(oldSchema, `"name":"atk"`, `"name":"attack"`, 1)
	_, problems := run(t, fx, next) // op 없이 이름만 바뀜 — 짐작하지 않는다
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "dropColumn") {
		t.Fatalf("op 없이 사라진 열을 막아야 한다: %v", problems)
	}
}

func TestDropColumnCountsLostValues(t *testing.T) {
	fx := load(t)
	next := strings.Replace(oldSchema, `,
  {"name":"note","type":"string","default":""}`, ``, 1)
	r := mustRun(t, fx, next, Op{Op: OpDropColumn, Table: "item", Column: "note"})
	if r.Changes["item"].ValuesLost != 1 {
		t.Fatalf("잃는 값이 1 이어야 한다: %+v", r.Changes["item"])
	}
	if strings.Contains(formatted(t, r, next), "note") {
		t.Fatal("지운 열이 남았다")
	}
}

func TestRenameEnumValueFollowsRowsListsAndDefault(t *testing.T) {
	fx := load(t)
	// 웹은 값 목록만 고치고 default 는 옛 그대로 보낸다 — 서버가 따라 고친다.
	next := strings.Replace(oldSchema, `"rare","epic"]`, `"uncommon","epic"]`, 1)
	r := mustRun(t, fx, next, Op{Op: OpRenameEnumValue, Enum: "Grade", From: "rare", To: "uncommon"})
	if got := string(r.Schema.Table("item").Column("grade").Default); got != `"uncommon"` {
		t.Fatalf("default 를 안 따라 고쳤다: %s", got)
	}
	got := formatted(t, r, next)
	if !strings.Contains(got, `"tags":["uncommon","epic"]`) {
		t.Fatalf("list<enum> 값을 안 고쳤다:\n%s", got)
	}
	// 기본값으로 rare 였던 c 행은 여전히 기본값(uncommon)이다 — 파일에 안 적힌다.
	if !strings.Contains(got, `{"id":"c"}`) {
		t.Fatalf("기본값 행이 바뀌었다:\n%s", got)
	}
}

func TestDropUsedEnumValueNeedsReplace(t *testing.T) {
	fx := load(t)
	next := strings.Replace(oldSchema, `,"epic"]`, `]`, 1)
	_, problems := run(t, fx, next, Op{Op: OpDropEnumValue, Enum: "Grade", Value: "epic"})
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "replaceWith") {
		t.Fatalf("쓰는 값을 대체 없이 지우면 막아야 한다: %v", problems)
	}

	r := mustRun(t, fx, next, Op{Op: OpDropEnumValue, Enum: "Grade", Value: "epic", ReplaceWith: "rare"})
	got := formatted(t, r, next)
	if !strings.Contains(got, `{"id":"a","atk":3,"tags":["rare","rare"],"note":"x"}`) {
		t.Fatalf("대체 값으로 안 바꿨다:\n%s", got)
	}
}

func TestDropUnusedEnumValue(t *testing.T) {
	fx := load(t)
	fx.tables["item"].Rows = fx.tables["item"].Rows[1:] // epic 을 쓰는 a 행을 뺀다
	next := strings.Replace(oldSchema, `,"epic"]`, `]`, 1)
	mustRun(t, fx, next, Op{Op: OpDropEnumValue, Enum: "Grade", Value: "epic"})
}

func TestDroppedDefaultValueNeedsReplace(t *testing.T) {
	fx := load(t)
	fx.tables["item"].Rows = fx.tables["item"].Rows[1:]
	next := strings.Replace(oldSchema, `"common","rare",`, `"common",`, 1)
	_, problems := run(t, fx, next, Op{Op: OpDropEnumValue, Enum: "Grade", Value: "rare"})
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "기본값") {
		t.Fatalf("기본값이 지운 값이면 막아야 한다: %v", problems)
	}
}

func TestRenameEnumThenValue(t *testing.T) {
	fx := load(t)
	next := strings.ReplaceAll(oldSchema, `Grade`, `Rank`)
	next = strings.Replace(next, `"epic"]`, `"legend"]`, 1)
	r := mustRun(t, fx, next,
		Op{Op: OpRenameEnum, From: "Grade", To: "Rank"},
		Op{Op: OpRenameEnumValue, Enum: "Rank", From: "epic", To: "legend"})
	if !strings.Contains(formatted(t, r, next), `"grade":"legend"`) {
		t.Fatal("enum 이름을 바꾼 뒤 값 바꾸기가 안 먹었다")
	}
}

func TestBadOps(t *testing.T) {
	fx := load(t)
	cases := map[string]Op{
		"모르는 op":  {Op: "nope"},
		"없는 표":    {Op: OpRenameColumn, Table: "zzz", From: "atk", To: "b"},
		"없는 열":    {Op: OpRenameColumn, Table: "item", From: "zzz", To: "b"},
		"id 바꾸기":  {Op: OpRenameColumn, Table: "item", From: "id", To: "key"},
		"이미 있는 열": {Op: OpRenameColumn, Table: "item", From: "atk", To: "note"},
		"이름 꼴":    {Op: OpRenameColumn, Table: "item", From: "atk", To: "Bad Name"},
		"없는 enum": {Op: OpRenameEnum, From: "Zzz", To: "Y"},
		"없는 값":    {Op: OpRenameEnumValue, Enum: "Grade", From: "zzz", To: "y"},
		"이미 있는 값": {Op: OpRenameEnumValue, Enum: "Grade", From: "rare", To: "epic"},
		"자기로 대체":  {Op: OpDropEnumValue, Enum: "Grade", Value: "rare", ReplaceWith: "rare"},
		"칸 빔":     {Op: OpDropColumn, Table: "item"},
	}
	for name, op := range cases {
		if _, problems := New(fx.old, []Op{op}); len(problems) == 0 {
			t.Errorf("%s: 막아야 한다", name)
		}
	}
}

func TestTableSetChangeRefused(t *testing.T) {
	fx := load(t)
	next := strings.Replace(oldSchema, `"name":"item"`, `"name":"thing"`, 1)
	_, problems := run(t, fx, next)
	if len(problems) == 0 {
		t.Fatal("표 이름 바꾸기는 2판 몫이라 막아야 한다")
	}
}

func TestNotesForNumberAndDefaultChange(t *testing.T) {
	fx := load(t)
	next := strings.Replace(oldSchema, `["common","rare","epic"]`, `{"common":0,"epic":2}`, 1)
	next = strings.Replace(next, `"default":0`, `"default":5`, 1)
	r := mustRun(t, fx, next, Op{Op: OpDropEnumValue, Enum: "Grade", Value: "rare", ReplaceWith: "common"})
	all := strings.Join(r.Notes, "\n")
	if strings.Contains(all, "C# 숫자") {
		t.Fatalf("숫자를 지켰는데 숫자 경고가 났다: %s", all)
	}
	if !strings.Contains(all, "item.atk") || !strings.Contains(all, "2") {
		t.Fatalf("기본값 바뀜 경고가 없다: %s", all)
	}

	shifted := strings.Replace(oldSchema, `"common","rare",`, `"common",`, 1)
	r = mustRun(t, fx, shifted, Op{Op: OpDropEnumValue, Enum: "Grade", Value: "rare", ReplaceWith: "common"})
	if !strings.Contains(strings.Join(r.Notes, "\n"), "C# 숫자") {
		t.Fatalf("배열 꼴에서 값을 지워 숫자가 밀렸는데 경고가 없다: %v", r.Notes)
	}
}
