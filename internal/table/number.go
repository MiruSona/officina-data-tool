package table

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
)

// errNumberRange 는 float64 로 못 담는 수(1e999 등)다. fmt 는 이 오류의 파일:줄을 알리고 멈춘다.
var errNumberRange = errors.New("수가 float64 범위를 넘었다")

// normalizeNumbers 는 디코드한 값 나무를 돌며 json.Number 를 JS 꼴 철자로 바꾼다.
// 맵·배열 안까지 같은 규칙이다 (설계 2026-10-03 5장 #17).
func normalizeNumbers(v any) (any, error) {
	switch x := v.(type) {
	case json.Number:
		s, err := jsNumber(string(x))
		if err != nil {
			return nil, err
		}
		return json.Number(s), nil
	case []any:
		for i, e := range x {
			n, err := normalizeNumbers(e)
			if err != nil {
				return nil, err
			}
			x[i] = n
		}
		return x, nil
	case map[string]any:
		for k, e := range x {
			n, err := normalizeNumbers(e)
			if err != nil {
				return nil, err
			}
			x[k] = n
		}
		return x, nil
	default:
		return v, nil
	}
}

// jsNumber 는 JSON 숫자 글을 float64 로 읽어 ECMAScript Number::toString 꼴로 적는다.
// 브라우저가 JSON.stringify 로 보내는 꼴과 글자까지 같아서 fmt 와 웹 UI 의 diff 가 갈리지 않는다.
//
// Go 의 FormatFloat(v,'g',-1) 은 1e+20·1e-05 처럼 고정/지수 경계가 JS 와 달라 쓰지 않는다.
// 대신 'e' 로 최단 숫자열 s(k 자리)와 지수를 얻고, n = 지수+1 로 JS 규칙을 그대로 따른다.
func jsNumber(text string) (string, error) {
	v, err := strconv.ParseFloat(text, 64)
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return "", errNumberRange // ParseFloat 은 넘침에 ±Inf 와 ErrRange 를 같이 준다
	}
	if err != nil {
		return "", err // 문법 오류는 JSON 디코더가 먼저 막으므로 여기 오지 않는다
	}
	if v == 0 {
		return "0", nil // -0 도 0 이다 (JS String(-0))
	}

	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	e := strconv.FormatFloat(v, 'e', -1, 64) // 예: "1.25e+01"
	mant, expText, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(expText)
	digits := strings.Replace(mant, ".", "", 1)
	k := len(digits)
	n := exp + 1

	var out string
	switch {
	case k <= n && n <= 21:
		out = digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		out = digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		out = "0." + strings.Repeat("0", -n) + digits
	default:
		expSign := "+"
		if n-1 < 0 {
			expSign = "-"
		}
		absExp := strconv.Itoa(abs(n - 1))
		if k == 1 {
			out = digits + "e" + expSign + absExp
		} else {
			out = digits[:1] + "." + digits[1:] + "e" + expSign + absExp
		}
	}
	return sign + out, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
