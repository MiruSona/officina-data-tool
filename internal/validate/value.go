package validate

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

// isNull 은 값이 JSON null 인지 본다.
//
// **null 을 먼저 막는 이유** : encoding/json 은 null 을 string·bool·숫자에 넣어도
// 오류를 안 낸다 — 그냥 아무것도 안 하고 지나간다. 그래서 `{"name":null}` 이
// 조용히 `""` 로, `{"usable":null}` 이 `false` 로 구워진다. null 은 값이 아니다.
func isNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

// asNumber 는 값이 JSON 숫자면 그 값을 준다. 문자열 "12" 도 null 도 숫자가 아니다.
func asNumber(raw json.RawMessage) (float64, bool) {
	if isNull(raw) {
		return 0, false
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, false
	}
	return v, true
}

func asString(raw json.RawMessage) (string, bool) {
	if isNull(raw) {
		return "", false
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	return v, true
}

func isBool(raw json.RawMessage) bool {
	if isNull(raw) {
		return false
	}
	var v bool
	return json.Unmarshal(raw, &v) == nil
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

// nearest 는 오타로 보이는 id 중 가장 가까운 것을 준다. 편집 거리 2까지만 본다 (설계 6장).
// 더 멀리 보면 엉뚱한 것을 자신 있게 권하게 되어 오히려 헷갈린다.
func nearest(want string, ids map[string]int) string {
	const limit = 2
	best, bestDistance := "", limit+1
	candidates := make([]string, 0, len(ids))
	for id := range ids {
		candidates = append(candidates, id)
	}
	sort.Strings(candidates) // 같은 거리면 늘 같은 것을 고르게 한다
	for _, id := range candidates {
		if d := distance(want, id, limit); d < bestDistance {
			best, bestDistance = id, d
		}
	}
	return best
}

// distance 는 편집 거리다. limit 를 넘으면 limit+1 을 준다 (거기서부터는 값이 같으므로 안 센다).
func distance(a, b string, limit int) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra)-len(rb) > limit || len(rb)-len(ra) > limit {
		return limit + 1
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	if prev[len(rb)] > limit {
		return limit + 1
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
