package schema_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mirusona/officina-data-tool/internal/schema"
)

// 시험용 열. Type 은 오류 문구에만 쓰인다.
func scalarCol(base string) *schema.Column {
	col := &schema.Column{Name: "v", Type: base, Base: base}
	if base == schema.TypeEnum {
		col.Enum = "Grade"
	}
	if base == schema.TypeRef {
		col.Ref = "item"
	}
	return col
}

func listCol(base string) *schema.Column {
	col := scalarCol(base)
	col.Type = "list<" + base + ">"
	col.IsList = true
	return col
}

var valueFile = &schema.File{Enums: map[string][]string{"Grade": {"common", "rare", "epic"}}}

// 설계 2-3 표 한 줄 = 한 칸. 받는 꼴은 결과 Go 값까지, 거절 꼴은 Kind 와 문구 토막까지 본다.
func TestDecodeItemTable(t *testing.T) {
	type tc struct {
		name string
		col  *schema.Column
		raw  string
		want any              // 받을 때 결과
		kind schema.ValueKind // 거절할 때
		text string           // 거절 문구 (글자 그대로)
	}
	cases := []tc{
		// int
		{"int 받음", scalarCol("int"), `12`, int64(12), "", ""},
		{"int 음수 받음", scalarCol("int"), `-2147483648`, int64(-2147483648), "", ""},
		{"int 문자열 type", scalarCol("int"), `"12"`, nil, schema.BadType, `int 가 와야 하는데 문자열 "12" 다`},
		{"int 소수 꼴 form", scalarCol("int"), `12.0`, nil, schema.BadForm, "int 열인데 소수 꼴로 적혔다: 12.0 (소수점 없이 적는다)"},
		{"int 지수 꼴 form", scalarCol("int"), `1e3`, nil, schema.BadForm, "int 열인데 지수 꼴로 적혔다: 1e3 (자릿수를 다 적는다)"},
		{"int 대문자 지수 꼴 form", scalarCol("int"), `1E3`, nil, schema.BadForm, "int 열인데 지수 꼴로 적혔다: 1E3 (자릿수를 다 적는다)"},
		{"int float64 밖 지수 꼴 form", scalarCol("int"), `1e400`, nil, schema.BadForm, "int 열인데 지수 꼴로 적혔다: 1e400 (자릿수를 다 적는다)"},
		{"int 소수 값 form", scalarCol("int"), `12.5`, nil, schema.BadForm, "int 열인데 소수다: 12.5"},
		{"int 지수로 쓴 소수 값 form", scalarCol("int"), `1.5e-1`, nil, schema.BadForm, "int 열인데 소수다: 1.5e-1"},
		{"int32 밖 range", scalarCol("int"), `2147483648`, nil, schema.BadRange, "int 가 담을 수 있는 범위를 넘었다: 2147483648 (C# int 는 32비트다)"},
		{"int64 밖 range", scalarCol("int"), `99999999999999999999`, nil, schema.BadRange, "int 가 담을 수 있는 범위를 넘었다: 99999999999999999999 (C# int 는 32비트다)"},
		{"int null", scalarCol("int"), `null`, nil, schema.BadNull, "int 가 와야 하는데 널 null 다"},
		// float
		{"float 받음", scalarCol("float"), `1.5`, float32(1.5), "", ""},
		{"float 정수 꼴 받음", scalarCol("float"), `3`, float32(3), "", ""},
		{"float 비정규수 받음", scalarCol("float"), `1e-40`, float32(1e-40), "", ""},
		{"float 0 받음", scalarCol("float"), `0`, float32(0), "", ""},
		{"float 0 지수 꼴 받음", scalarCol("float"), `0.0e-400`, float32(0), "", ""},
		{"float 문자열 type", scalarCol("float"), `"1.5"`, nil, schema.BadType, `float 가 와야 하는데 문자열 "1.5" 다`},
		{"float32 넘침 range", scalarCol("float"), `1e39`, nil, schema.BadRange, "float32 로 담을 수 없다: 1e39 (C# float 은 32비트다)"},
		{"float32 음수 넘침 range", scalarCol("float"), `-1e39`, nil, schema.BadRange, "float32 로 담을 수 없다: -1e39 (C# float 은 32비트다)"},
		{"float64 넘침 range", scalarCol("float"), `1e400`, nil, schema.BadRange, "float32 로 담을 수 없다: 1e400 (C# float 은 32비트다)"},
		{"float32 아래넘침 range", scalarCol("float"), `1e-50`, nil, schema.BadRange, "float32 로 담으면 0 이 된다: 1e-50 (0 이면 0 으로 적는다)"},
		{"float64 아래넘침 range", scalarCol("float"), `1e-400`, nil, schema.BadRange, "float32 로 담으면 0 이 된다: 1e-400 (0 이면 0 으로 적는다)"},
		{"float null", scalarCol("float"), `null`, nil, schema.BadNull, "float 가 와야 하는데 널 null 다"},
		// bool
		{"bool 참 받음", scalarCol("bool"), `true`, true, "", ""},
		{"bool 거짓 받음", scalarCol("bool"), `false`, false, "", ""},
		{"bool 수 type", scalarCol("bool"), `1`, nil, schema.BadType, "bool 이 와야 하는데 숫자 1 다"},
		{"bool null", scalarCol("bool"), `null`, nil, schema.BadNull, "bool 이 와야 하는데 널 null 다"},
		// string
		{"string 받음", scalarCol("string"), `"abc"`, "abc", "", ""},
		{"string 빈 값 받음", scalarCol("string"), `""`, "", "", ""},
		{"string 수 type", scalarCol("string"), `12`, nil, schema.BadType, "string 이 와야 하는데 숫자 12 다"},
		{"string null", scalarCol("string"), `null`, nil, schema.BadNull, "string 이 와야 하는데 널 null 다"},
		// enum
		{"enum 받음", scalarCol("enum"), `"rare"`, "rare", "", ""},
		{"enum 수 type", scalarCol("enum"), `1`, nil, schema.BadType, "enum 이 와야 하는데 숫자 1 다"},
		{"enum 목록 밖", scalarCol("enum"), `"legend"`, nil, schema.BadEnum, `enum Grade 에 없는 값이다: "legend" (있는 것: common, rare, epic)`},
		{"enum null", scalarCol("enum"), `null`, nil, schema.BadNull, "enum 이 와야 하는데 널 null 다"},
		// ref · asset
		{"ref 받음", scalarCol("ref"), `"sword"`, "sword", "", ""},
		{"ref 빈 값 받음 (스칼라)", scalarCol("ref"), `""`, "", "", ""},
		{"ref 수 type", scalarCol("ref"), `3`, nil, schema.BadType, "ref 가 와야 하는데 숫자 3 다"},
		{"ref null", scalarCol("ref"), `null`, nil, schema.BadNull, "ref 가 와야 하는데 널 null 다"},
		{"asset 받음", scalarCol("asset"), `"Sfx/hit.wav"`, "Sfx/hit.wav", "", ""},
		{"asset 빈 값 받음 (스칼라)", scalarCol("asset"), `""`, "", "", ""},
		{"asset 참거짓 type", scalarCol("asset"), `true`, nil, schema.BadType, "asset 이 와야 하는데 참거짓 true 다"},
		// list 원소
		{"list<ref> 원소 받음", listCol("ref"), `"sword"`, "sword", "", ""},
		{"list<ref> 원소 빈 id", listCol("ref"), `""`, nil, schema.BadList, "list<ref> 안에 빈 id 가 있다 (빼거나 id 를 적는다)"},
		{"list<asset> 원소 빈 주소", listCol("asset"), `""`, nil, schema.BadList, "list<asset> 안에 빈 주소가 있다 (빼거나 주소를 적는다)"},
		{"list<string> 원소 빈 값 받음", listCol("string"), `""`, "", "", ""},
		{"list<int> 원소 null", listCol("int"), `null`, nil, schema.BadNull, "int 가 와야 하는데 널 null 다"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := schema.DecodeItem(valueFile, c.col, json.RawMessage(c.raw))
			checkDecoded(t, got, err, c.want, c.kind, c.text)
			if c.kind != "" && err.Error() != c.text {
				t.Fatalf("문구가 다르다\n  얻음: %q\n  기대: %q", err.Error(), c.text)
			}
		})
	}
}

func checkDecoded(t *testing.T, got any, err error, want any, kind schema.ValueKind, text string) {
	t.Helper()
	if kind == "" {
		if err != nil {
			t.Fatalf("받아야 하는데 거절했다: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("결과가 %#v (%T) 인데 %#v (%T) 여야 한다", got, got, want, want)
		}
		return
	}
	var ve *schema.ValueError
	if !errors.As(err, &ve) {
		t.Fatalf("*ValueError 가 아니다: %v", err)
	}
	if ve.Kind != kind {
		t.Fatalf("Kind 가 %q 인데 %q 여야 한다 (%s)", ve.Kind, kind, ve.Message)
	}
	if !strings.Contains(ve.Message, text) {
		t.Fatalf("문구에 %q 가 없다: %q", text, ve.Message)
	}
}

// 설계 2-3 「list<T>」 줄 : 배열은 원소로 풀고, null·배열 아님은 BadList.
func TestItems(t *testing.T) {
	col := listCol("int")
	items, err := schema.Items(col, json.RawMessage(`[1, 2]`))
	if err != nil || len(items) != 2 || string(items[1]) != "2" {
		t.Fatalf("배열을 못 풀었다: %v %v", items, err)
	}
	empty, err := schema.Items(col, json.RawMessage(`[]`))
	if err != nil || len(empty) != 0 {
		t.Fatalf("빈 배열을 못 받았다: %v %v", empty, err)
	}
	for _, raw := range []string{`null`, `"a"`, `3`} {
		_, err := schema.Items(col, json.RawMessage(raw))
		checkDecoded(t, nil, err, nil, schema.BadList, "list<int> 열인데 배열이 아니다:")
	}
}

// Decode 는 list 를 통째로 []any 로, 원소 오류는 「%d번째 원소가 틀렸다」로 감싼다.
func TestDecode(t *testing.T) {
	got, err := schema.Decode(valueFile, listCol("int"), json.RawMessage(`[1, 2]`))
	checkDecoded(t, got, err, []any{int64(1), int64(2)}, "", "")

	empty, err := schema.Decode(valueFile, listCol("string"), json.RawMessage(`[]`))
	checkDecoded(t, empty, err, []any{}, "", "")

	scalar, err := schema.Decode(valueFile, scalarCol("enum"), json.RawMessage(`"epic"`))
	checkDecoded(t, scalar, err, "epic", "", "")

	_, err = schema.Decode(valueFile, listCol("int"), json.RawMessage(`[1, 12.0]`))
	checkDecoded(t, nil, err, nil, schema.BadForm, "1번째 원소가 틀렸다 : int 열인데 소수 꼴로 적혔다")

	_, err = schema.Decode(valueFile, listCol("ref"), json.RawMessage(`[""]`))
	checkDecoded(t, nil, err, nil, schema.BadList, "0번째 원소가 틀렸다 : list<ref> 안에 빈 id")

	_, err = schema.Decode(valueFile, listCol("int"), json.RawMessage(`null`))
	checkDecoded(t, nil, err, nil, schema.BadList, "배열이 아니다")
}

// 없는 enum 은 checkColumnEnum 이 알리므로 목록 확인을 건너뛴다. 스키마가 nil 이어도 같다.
func TestDecodeUnknownEnumSkipsList(t *testing.T) {
	col := scalarCol("enum")
	col.Enum = "Nope"
	got, err := schema.DecodeItem(valueFile, col, json.RawMessage(`"anything"`))
	checkDecoded(t, got, err, "anything", "", "")
	got, err = schema.DecodeItem(nil, scalarCol("enum"), json.RawMessage(`"anything"`))
	checkDecoded(t, got, err, "anything", "", "")
}

// Decode 가 감싼 오류도 *ValueError 이고 Kind 를 지킨다 — 단언이 아니라 errors.As 로 꺼낸다.
func TestDecodeWrapsKind(t *testing.T) {
	_, err := schema.Decode(valueFile, listCol("float"), json.RawMessage(`[1, 1e-400]`))
	checkDecoded(t, nil, err, nil, schema.BadRange, "1번째 원소가 틀렸다 : float32 로 담으면 0 이 된다: 1e-400")
}

// 수가 아닌 값에 CheckBounds 를 부르면 잘못 부른 것이다 — 조용히 통과시키지 않고 BadType 을 준다.
func TestCheckBoundsRejectsNonNumber(t *testing.T) {
	for _, raw := range []string{`"5"`, `null`, `true`} {
		err := schema.CheckBounds(scalarCol("int"), json.RawMessage(raw))
		checkDecoded(t, nil, err, nil, schema.BadType, "min·max 는 수에만 견준다")
	}
}

// min·max 는 raw 를 float64 로 견준다 — float32 로 낮춰 견주면 max 0.1 에 0.1 이 거짓 오류다.
func TestCheckBounds(t *testing.T) {
	lo, hi := 1.0, 0.1
	col := scalarCol("float")
	col.Max = &hi
	if err := schema.CheckBounds(col, json.RawMessage(`0.1`)); err != nil {
		t.Fatalf("max 0.1 에 0.1 을 거절했다: %v", err)
	}
	err := schema.CheckBounds(col, json.RawMessage(`0.2`))
	var ve *schema.ValueError
	if !errors.As(err, &ve) || ve.Message != "max 0.1 보다 크다: 0.2" {
		t.Fatalf("max 밖을 못 잡았다: %v", err)
	}

	ints := scalarCol("int")
	ints.Min = &lo
	err = schema.CheckBounds(ints, json.RawMessage(`0`))
	if !errors.As(err, &ve) || ve.Message != "min 1 보다 작다: 0" {
		t.Fatalf("min 밖을 못 잡았다: %v", err)
	}
	if err := schema.CheckBounds(ints, json.RawMessage(`1`)); err != nil {
		t.Fatalf("min 과 같은 값을 거절했다: %v", err)
	}
	if err := schema.CheckBounds(scalarCol("int"), json.RawMessage(`-5`)); err != nil {
		t.Fatalf("min·max 없는 열이 거절했다: %v", err)
	}
}
