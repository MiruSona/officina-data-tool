package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// 「JSON 값 하나가 이 열에 맞나」를 판정하는 한 곳이다. 스키마 default 검사 · validate · bake 가 다 부른다.
// 판정 표와 문구는 Docs/Design/2026-10-12-값변환기합치기설계.md 2장.

// ValueKind 는 값이 틀린 갈래다. Column.Kind(asset kind)와 다른 것이다.
type ValueKind string

const (
	BadType  ValueKind = "type"  // 타입이 아니다
	BadForm  ValueKind = "form"  // 값은 맞는데 적은 꼴이 틀렸다 (int 열에 12.0)
	BadRange ValueKind = "range" // 타입이 못 담는다 (int32 · float32). min·max 가 아니다
	BadEnum  ValueKind = "enum"  // enum 목록 밖
	BadNull  ValueKind = "null"  // null 은 값이 아니다
	BadList  ValueKind = "list"  // 배열이 아니다 · list<ref>/list<asset> 안에 ""
)

// ValueError 는 값 하나의 잘못이다. Message 는 「파일:줄: 표.열 — 」 뒤에 붙는 부분만 담는다.
type ValueError struct {
	Kind    ValueKind
	Message string
}

func (e *ValueError) Error() string { return e.Message }

func valueError(kind ValueKind, format string, args ...any) *ValueError {
	return &ValueError{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// intForm 은 int 열에 적을 수 있는 글자 꼴이다 — `12` 는 되고 `12.0`·`1e3` 은 안 된다.
var intForm = regexp.MustCompile(`^-?[0-9]+$`)

// Decode 는 칸 하나(list 면 배열 통째)를 굽는 Go 값으로 바꾼다. 첫 잘못에서 멈춘다. bake 가 쓴다.
func Decode(f *File, col *Column, raw json.RawMessage) (any, error) {
	if !col.IsList {
		return DecodeItem(f, col, raw)
	}
	items, err := Items(col, raw)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(items))
	for i, item := range items {
		v, err := DecodeItem(f, col, item)
		if err != nil {
			kind := BadType // DecodeItem 은 늘 *ValueError 를 주지만, 아니어도 Kind 없이 터지지 않게 둔다
			var inner *ValueError
			if errors.As(err, &inner) {
				kind = inner.Kind
			}
			return nil, valueError(kind, "%d번째 원소가 틀렸다 : %s", i, err.Error())
		}
		out = append(out, v)
	}
	return out, nil
}

// Items 는 list 열의 배열을 원소로 푼다. null·배열 아님은 BadList.
//
// null 을 먼저 막는 까닭 : encoding/json 은 null 을 조각에 넣어도 오류를 안 내고 nil 조각을 준다.
func Items(col *Column, raw json.RawMessage) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if isNullValue(raw) || json.Unmarshal(raw, &items) != nil {
		return nil, valueError(BadList, "%s 열인데 배열이 아니다: %s", col.Type, describe(raw))
	}
	return items, nil
}

// DecodeItem 은 원소 하나를 col.Base 로 바꾼다. list 열이면 「list 안 빈 값」도 본다.
// 원소마다 따로 알려야 하는 쪽(스키마 default · validate)이 쓴다. 차례는 null → 타입 → 꼴 → 범위 → 목록.
func DecodeItem(f *File, col *Column, raw json.RawMessage) (any, error) {
	// encoding/json 은 null 을 string·bool·숫자에 넣어도 오류를 안 낸다 — 조용히 ""·false·0 이 된다.
	if isNullValue(raw) {
		return nil, typeError(BadNull, col, raw)
	}
	switch col.Base {
	case TypeInt:
		return decodeInt(col, raw)
	case TypeFloat:
		return decodeFloat(col, raw)
	case TypeBool:
		var v bool
		if json.Unmarshal(raw, &v) != nil {
			return nil, typeError(BadType, col, raw)
		}
		return v, nil
	case TypeString:
		return decodeString(col, raw)
	case TypeEnum:
		return decodeEnum(f, col, raw)
	case TypeRef, TypeAsset:
		return decodeAddress(col, raw)
	}
	return nil, valueError(BadType, "모르는 타입 %q 다", col.Type)
}

// decodeInt 는 정수 글자만 받는다. C# 쪽이 int(32비트)라 그 범위도 본다. 결과는 int64 다 (구운 바이트가 그대로다).
// 판정은 raw 글자로 한다 — encoding/json 의 float64 는 1e400 을 오류로, 큰 정수를 반올림해서 준다.
func decodeInt(col *Column, raw json.RawMessage) (any, error) {
	text, ok := numberText(raw)
	if !ok {
		return nil, typeError(BadType, col, raw)
	}
	if intForm.MatchString(text) {
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n < math.MinInt32 || n > math.MaxInt32 {
			return nil, valueError(BadRange, "int 가 담을 수 있는 범위를 넘었다: %s (C# int 는 32비트다)", text)
		}
		return n, nil
	}
	// 정수 글자가 아니다. 값이 소수면 소수라고, 값은 정수인데 적은 꼴이 틀렸으면 꼴을 말한다.
	v, err := strconv.ParseFloat(text, 64)
	if err == nil && v != math.Trunc(v) {
		return nil, valueError(BadForm, "int 열인데 소수다: %s", text)
	}
	if strings.ContainsAny(text, "eE") {
		return nil, valueError(BadForm, "int 열인데 지수 꼴로 적혔다: %s (자릿수를 다 적는다)", text)
	}
	return nil, valueError(BadForm, "int 열인데 소수 꼴로 적혔다: %s (소수점 없이 적는다)", text)
}

// decodeFloat 는 float32 로 낮춘다. 판정은 raw 글자로 한다 — encoding/json 은 1e-400 을 오류 없이 0 으로,
// 1e400 은 「수 아님」으로 준다. 1e39 는 float32 에서 무한대, 1e-50 은 0 이 되니 막는다. 1e-40 같은 비정규수는 받는다.
func decodeFloat(col *Column, raw json.RawMessage) (any, error) {
	text, ok := numberText(raw)
	if !ok {
		return nil, typeError(BadType, col, raw)
	}
	overflow := valueError(BadRange, "float32 로 담을 수 없다: %s (C# float 은 32비트다)", text)
	v64, err := strconv.ParseFloat(text, 64)
	if err != nil || v64 < -math.MaxFloat32 || v64 > math.MaxFloat32 {
		return nil, overflow // float64 로 못 담는 것(오류)은 무한대 쪽뿐이다 — 아래넘침은 오류 없이 0 이다
	}
	f32, err := strconv.ParseFloat(text, 32)
	if err != nil {
		return nil, overflow
	}
	if f32 == 0 && nonZeroMantissa(text) {
		return nil, valueError(BadRange, "float32 로 담으면 0 이 된다: %s (0 이면 0 으로 적는다)", text)
	}
	return float32(f32), nil
}

// numberText 는 raw 가 JSON 수일 때만 그 글자를 준다. 문자열 "12" 도 null 도 수가 아니다.
func numberText(raw json.RawMessage) (string, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" || !json.Valid([]byte(text)) {
		return "", false
	}
	if text[0] != '-' && (text[0] < '0' || text[0] > '9') {
		return "", false
	}
	return text, true
}

// nonZeroMantissa 는 지수 앞 자리에 0 아닌 숫자가 있는지 본다. 0.0e-400 은 진짜 0 이고 1e-400 은 아니다.
func nonZeroMantissa(text string) bool {
	mantissa := text
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		mantissa = text[:i]
	}
	return strings.ContainsAny(mantissa, "123456789")
}

func decodeString(col *Column, raw json.RawMessage) (string, error) {
	var v string
	if json.Unmarshal(raw, &v) != nil {
		return "", typeError(BadType, col, raw)
	}
	return v, nil
}

// decodeEnum 은 이름 문자열로 받는다. 없는 enum 이면 목록 확인을 건너뛴다 — checkColumnEnum 이 이미 알린다.
func decodeEnum(f *File, col *Column, raw json.RawMessage) (any, error) {
	v, err := decodeString(col, raw)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return v, nil
	}
	values, found := f.EnumValues(col.Enum)
	if !found || contains(values, v) {
		return v, nil
	}
	return nil, valueError(BadEnum, "enum %s 에 없는 값이다: %q (있는 것: %s)",
		col.Enum, v, strings.Join(values, ", "))
}

// decodeAddress 는 ref·asset 원소다. 스칼라의 ""는 「없음」이라 받고 (필수 열인지는 validate 가 본다),
// list 안의 ""는 빈 자리라 막는다.
func decodeAddress(col *Column, raw json.RawMessage) (any, error) {
	v, err := decodeString(col, raw)
	if err != nil {
		return nil, err
	}
	if v != "" || !col.IsList {
		return v, nil
	}
	if col.Base == TypeRef {
		return nil, valueError(BadList, "list<ref> 안에 빈 id 가 있다 (빼거나 id 를 적는다)")
	}
	return nil, valueError(BadList, "list<asset> 안에 빈 주소가 있다 (빼거나 주소를 적는다)")
}

// CheckBounds 는 min·max 안인지 본다. Decode 가 통과한 int·float 값에만 부른다.
// float32 로 낮춘 값이 아니라 raw 를 float64 로 견준다 — max 0.1 에 0.1 이 거짓 오류가 안 나게.
// 오류 Kind 는 BadRange 지만 부른 자리가 곧 min·max 검사라 부르는 쪽은 Kind 를 안 본다.
// 수가 아니면 잘못 부른 것이라 BadType 을 준다 — 조용히 통과시키지 않는다.
func CheckBounds(col *Column, raw json.RawMessage) error {
	text, ok := numberText(raw)
	if !ok {
		return valueError(BadType, "min·max 는 수에만 견준다: %s", describe(raw))
	}
	v, _ := strconv.ParseFloat(text, 64) // 범위 밖이면 ±Inf·0 이 와서 그대로 견주면 된다
	if col.Min != nil && v < *col.Min {
		return valueError(BadRange, "min %g 보다 작다: %g", *col.Min, v)
	}
	if col.Max != nil && v > *col.Max {
		return valueError(BadRange, "max %g 보다 크다: %g", *col.Max, v)
	}
	return nil
}

// typeError 는 「int 가 와야 하는데 문자열 "12" 다」 꼴이다. null 도 같은 꼴로 쓴다.
func typeError(kind ValueKind, col *Column, raw json.RawMessage) *ValueError {
	return valueError(kind, "%s%s 와야 하는데 %s 다", col.Base, particle(col.Base), describe(raw))
}

// isNullValue 는 값이 JSON null 인지 본다.
func isNullValue(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

// particle 은 타입 이름 뒤에 붙는 조사다. 「string 가 와야 한다」가 눈에 걸려서 둔다.
func particle(base string) string {
	switch base {
	case TypeInt, TypeFloat, TypeRef:
		return " 가"
	}
	return " 이"
}

// describe 는 값을 사람이 읽는 한 토막으로 만든다 — `문자열 "12"` 처럼.
// 긴 값은 잘라낸다. 오류 한 건이 한 줄을 넘지 않게 하려는 것이다.
func describe(raw json.RawMessage) string {
	text := strings.TrimSpace(string(raw))
	if n := utf8.RuneCountInString(text); n > 40 {
		text = string([]rune(text)[:40]) + "…"
	}
	return kindOf(raw) + " " + text
}

func kindOf(raw json.RawMessage) string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "빈 값"
	}
	switch text[0] {
	case '"':
		return "문자열"
	case '[':
		return "배열"
	case '{':
		return "객체"
	case 't', 'f':
		return "참거짓"
	case 'n':
		return "널"
	}
	return "숫자"
}
