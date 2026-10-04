package schema

import (
	"strings"
	"testing"
)

// 시험 자료의 스키마 전부 — 꼴이 제각각인 손 글씨 파일들이다.
var sampleSchemas = []string{
	"../../Testdata/schema/ok/schema.json",
	"../../Testdata/validate/schema.json",
	"../../Testdata/asset/schema.json",
	"../../Testdata/table/ok/schema.json",
	"../../Testdata/table/messy/schema.json",
}

// Parse → Format → Parse 가 같은 뜻이고, 다시 Format 해도 글자가 같으며, 해시가 안 바뀐다.
func TestFormatRoundTrip(t *testing.T) {
	for _, path := range sampleSchemas {
		f, err := Load(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out := f.Format()
		again, err := Parse(out, path)
		if err != nil {
			t.Fatalf("%s: Format 한 것을 다시 못 읽는다: %v\n%s", path, err, out)
		}
		if again.Hash() != f.Hash() {
			t.Fatalf("%s: Format 뒤 해시가 바뀌었다", path)
		}
		if string(again.Format()) != string(out) {
			t.Fatalf("%s: Format 이 한 번에 안 굳는다", path)
		}
	}
}

// 옛 예제 스키마의 해시는 이번 판 전과 같아야 한다 (구운 파일이 안 깨진다).
func TestFormatKeepsKnownHash(t *testing.T) {
	f, err := Load("../../Testdata/schema/ok/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	// gen 골든(Testdata/gen/expected/GameDataTables.cs)에 박힌 값이다.
	const want = "1db5aa5f0ede6b0a"
	if got := f.Hash(); got != want {
		t.Fatalf("해시가 %s 여야 하는데 %s 다", want, got)
	}
}

func TestFormatShape(t *testing.T) {
	src := `{"tables":[{"columns":[{"type":"string","name":"id"},
	  {"default":"b","enum":"Grade","type":"enum","name":"grade","desc":"등급 <b>"},
	  {"name":"atk","type":"int","max":9999,"min":0,"default":0,"desc":"a\"b"}],"name":"item"}],
	  "enums":{"Grade":{"a":0,"b":3},"Element":["fire","ice"]},"namespace":"G.Data","version":1}`
	f, err := Parse([]byte(src), "s.json")
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "version": 1,
  "namespace": "G.Data",
  "enums": {
    "Element": ["fire", "ice"],
    "Grade": {"a": 0, "b": 3}
  },
  "tables": [
    { "name": "item", "columns": [
      { "name": "id", "type": "string" },
      { "name": "grade", "type": "enum", "enum": "Grade", "default": "b", "desc": "등급 <b>" },
      { "name": "atk", "type": "int", "min": 0, "max": 9999, "default": 0, "desc": "a\"b" }
    ] }
  ]
}
`
	if got := string(f.Format()); got != want {
		t.Fatalf("꼴이 다르다:\n%s", got)
	}
}

func TestEnumNumberForm(t *testing.T) {
	src := `{"version":1,"namespace":"G","enums":{"Grade":{"common":0,"rare":1,"epic":5},"Seq":{"a":0,"b":1}},
	  "tables":[{"name":"t","columns":[{"name":"id","type":"string"}]}]}`
	f, err := Parse([]byte(src), "s.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Enums["Grade"], ","); got != "common,rare,epic" {
		t.Fatalf("값 차례를 잃었다: %s", got)
	}
	nums := f.EnumNumbersOf("Grade")
	if len(nums) != 3 || nums[0] != 0 || nums[1] != 1 || nums[2] != 5 {
		t.Fatalf("숫자가 틀렸다: %v", nums)
	}
	// 0,1,… 차례 그대로인 객체 꼴은 배열 꼴로 적는다.
	if !strings.Contains(string(f.Format()), `"Seq": ["a", "b"]`) {
		t.Fatalf("차례 번호뿐인 enum 이 배열 꼴이 아니다:\n%s", f.Format())
	}
	// 배열 꼴은 차례 번호로 읽는다.
	arr, err := Parse([]byte(strings.Replace(src, `{"a":0,"b":1}`, `["a","b"]`, 1)), "s.json")
	if err != nil {
		t.Fatal(err)
	}
	if n := arr.EnumNumbersOf("Seq"); len(n) != 2 || n[0] != 0 || n[1] != 1 {
		t.Fatalf("배열 꼴 숫자가 차례 번호가 아니다: %v", n)
	}
}

// 숫자는 해시에 안 들어간다 (결정 4) — 숫자만 바꾼 스키마는 해시가 같다.
func TestEnumNumbersNotInHash(t *testing.T) {
	base := `{"version":1,"namespace":"G","enums":{"Grade":%s},"tables":[{"name":"t","columns":[{"name":"id","type":"string"}]}]}`
	a, err := Parse([]byte(strings.Replace(base, "%s", `["x","y"]`, 1)), "a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse([]byte(strings.Replace(base, "%s", `{"x":3,"y":7}`, 1)), "b")
	if err != nil {
		t.Fatal(err)
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("숫자가 해시에 들어갔다")
	}
}

func TestEnumNumberErrors(t *testing.T) {
	cases := map[string]string{
		"숫자 중복":  `{"a":1,"b":1}`,
		"음수":     `{"a":-1}`,
		"소수":     `{"a":1.5}`,
		"너무 큼":   `{"a":2147483648}`,
		"문자열 숫자": `{"a":"1"}`,
		"키 중복":   `{"a":0,"a":1}`,
		"빈 객체":   `{}`,
		"값 꼴":    `{"A":0}`,
	}
	base := `{"version":1,"namespace":"G","enums":{"Grade":%s},"tables":[{"name":"t","columns":[{"name":"id","type":"string"}]}]}`
	for name, enum := range cases {
		_, err := Parse([]byte(strings.Replace(base, "%s", enum, 1)), "s")
		if _, ok := err.(*Errors); !ok {
			t.Errorf("%s: 스키마 오류가 나야 한다 (%v)", name, err)
		}
	}
}
