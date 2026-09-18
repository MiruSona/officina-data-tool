package schema_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusona/officina-data-tool/internal/schema"
)

const okPath = "../../Testdata/schema/ok/schema.json"

// T1-a. 올바른 스키마는 통과하고 읽은 값이 그대로 들어 있다.
func TestLoadOK(t *testing.T) {
	f, err := schema.Load(okPath)
	if err != nil {
		t.Fatalf("올바른 스키마인데 실패했다: %v", err)
	}
	if f.Version != 1 || f.Namespace != "MyGame.Data" {
		t.Fatalf("머리 값이 틀리다: version=%d namespace=%q", f.Version, f.Namespace)
	}
	if got := f.TableNames(); strings.Join(got, ",") != "item,monster,drop" {
		t.Fatalf("표 차례가 적힌 대로가 아니다: %v", got)
	}
	if values, ok := f.EnumValues("Grade"); !ok || len(values) != 3 {
		t.Fatalf("enum Grade 를 못 읽었다: %v %v", values, ok)
	}
}

// T1-b. 열의 타입·기본값·차례를 제대로 풀었나.
func TestColumnDetail(t *testing.T) {
	f, err := schema.Load(okPath)
	if err != nil {
		t.Fatalf("읽기 실패: %v", err)
	}
	item := f.Table("item")
	if item == nil {
		t.Fatal("item 표가 없다")
	}
	if got := item.ColumnIndex("grade"); got != 5 {
		t.Fatalf("grade 열 차례가 5 여야 한다: %d", got)
	}
	tags := item.Column("tags")
	if !tags.IsList || tags.Base != schema.TypeString {
		t.Fatalf("list<string> 을 못 풀었다: IsList=%v Base=%q", tags.IsList, tags.Base)
	}
	if item.Column("atk").Required() {
		t.Fatal("기본값이 있는 atk 가 필수로 잡혔다")
	}
	if !f.Table("monster").Column("hp").Required() {
		t.Fatal("기본값이 없는 hp 가 필수가 아니다")
	}
	if !item.Column("name").Loc {
		t.Fatal("loc 표시를 못 읽었다")
	}
	if ref := f.Table("drop").Column("item_id").Ref; ref != "item" {
		t.Fatalf("ref 칸이 틀리다: %q", ref)
	}
}

// T1-c. 틀린 스키마는 저마다 알맞은 자리를 찍으며 *Errors 로 실패한다.
func TestBrokenSchemas(t *testing.T) {
	cases := map[string]string{
		"unknown-type":            "tables[0].columns[1].type",
		"missing-enum":            "tables[0].columns[1].enum",
		"first-column-not-id":     "tables[0].columns[0].name",
		"first-column-not-string": "tables[0].columns[0].type",
		// 첫 열 id 에는 꾸밈 칸을 못 붙인다 (리뷰 B).
		"first-column-with-default": "tables[0].columns[0].default",
		"first-column-with-ref":     "tables[0].columns[0].ref",
		"nested-list":               "tables[0].columns[1].type",
		"unknown-column-key":        "tables[0].columns[1].required",
		"bad-column-name":           "tables[0].columns[1].name",
		"missing-ref-table":         "tables[0].columns[1].ref",
		"duplicate-column":          "tables[0].columns[2].name",
		"default-type-mismatch":     "tables[0].columns[1].default",
		"enum-value-not-in-list":    "tables[0].columns[1].default",
		"min-greater-than-max":      "tables[0].columns[1]",
		"bounds-on-string":          "tables[0].columns[1]",
		"loc-on-int":                "tables[0].columns[1].loc",
		"bad-table-name":            "tables[0].name",
		"duplicate-table":           "tables[1].name",
		"no-namespace":              "namespace",
	}

	for name, where := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("../../Testdata/schema/broken", name+".json")
			_, err := schema.Load(path)
			if err == nil {
				t.Fatal("틀린 스키마인데 통과했다")
			}
			var se *schema.Errors
			if !errors.As(err, &se) {
				t.Fatalf("스키마 오류(*Errors)가 아니다: %v", err)
			}
			if !hasWhere(se, where) {
				t.Fatalf("자리 %q 를 못 찍었다:\n%v", where, err)
			}
		})
	}
}

// T1-d. JSON 자체가 깨진 것은 스키마 오류가 아니라 읽기 실패다 (종료 3 이 아니라 4).
func TestBrokenJSONIsNotSchemaError(t *testing.T) {
	_, err := schema.Load("../../Testdata/schema/broken/broken-json.json")
	if err == nil {
		t.Fatal("깨진 JSON 인데 통과했다")
	}
	var se *schema.Errors
	if errors.As(err, &se) {
		t.Fatalf("깨진 JSON 을 스키마 오류로 잡았다: %v", err)
	}
}

func TestMissingFile(t *testing.T) {
	_, err := schema.Load("../../Testdata/schema/없는파일.json")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("없는 파일인데 다른 오류다: %v", err)
	}
}

// T1-e. 오류를 첫 건에서 멈추지 않고 모은다.
func TestCollectsManyErrors(t *testing.T) {
	src := `{"version":1,"namespace":"My.Data","tables":[
		{"name":"Item","columns":[{"name":"id","type":"int"},{"name":"BadName","type":"date"}]}]}`
	_, err := schema.Parse([]byte(src), "schema.json")
	var se *schema.Errors
	if !errors.As(err, &se) {
		t.Fatalf("스키마 오류가 아니다: %v", err)
	}
	if len(se.List) < 4 {
		t.Fatalf("오류를 다 못 모았다 (%d건):\n%v", len(se.List), err)
	}
	if !strings.HasPrefix(se.Error(), "schema.json: ") {
		t.Fatalf("오류 줄이 파일 이름으로 시작하지 않는다:\n%v", se)
	}
}

// T1-f. schemaHash 는 들여쓰기·칸 차례에 안 흔들리고, 뜻이 바뀌면 바뀐다.
func TestHash(t *testing.T) {
	f, err := schema.Load(okPath)
	if err != nil {
		t.Fatalf("읽기 실패: %v", err)
	}
	if len(f.Hash()) != schema.HashLen {
		t.Fatalf("해시 길이가 %d 가 아니다: %q", schema.HashLen, f.Hash())
	}

	tight := `{"namespace":"My.Data","version":1,"tables":[{"columns":[{"type":"string","name":"id"}],"name":"item"}]}`
	spread := "{\n \"version\": 1,\n \"namespace\": \"My.Data\",\n \"tables\": [\n  { \"name\": \"item\",\n" +
		"    \"columns\": [ { \"name\": \"id\", \"type\": \"string\", \"desc\": \"열쇠\" } ] } ] }"
	a := mustParse(t, tight)
	b := mustParse(t, spread)
	if a.Hash() != b.Hash() {
		t.Fatalf("서식만 다른데 해시가 달라졌다: %s vs %s", a.Hash(), b.Hash())
	}

	changed := mustParse(t, `{"version":1,"namespace":"My.Data","tables":[{"name":"item","columns":[
		{"name":"id","type":"string"},{"name":"atk","type":"int","default":1}]}]}`)
	if changed.Hash() == a.Hash() {
		t.Fatal("열을 더했는데 해시가 그대로다")
	}
}

func mustParse(t *testing.T, src string) *schema.File {
	t.Helper()
	f, err := schema.Parse([]byte(src), "schema.json")
	if err != nil {
		t.Fatalf("읽기 실패: %v\n%s", err, src)
	}
	return f
}

func hasWhere(se *schema.Errors, where string) bool {
	for _, one := range se.List {
		if one.Where == where {
			return true
		}
	}
	return false
}

// BOM 이 붙은 schema.json 도 읽는다 (리뷰 I — 읽기 입구 셋이 같은 함수를 쓴다).
func TestLoadSchemaWithBOM(t *testing.T) {
	raw, err := os.ReadFile(okPath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, raw...), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := schema.Load(path)
	if err != nil {
		t.Fatalf("BOM 이 붙었다고 못 읽었다: %v", err)
	}
	if f.Namespace != "MyGame.Data" {
		t.Fatalf("namespace 가 %q 다", f.Namespace)
	}
}
