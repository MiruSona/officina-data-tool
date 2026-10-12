package validate

import (
	"encoding/json"
	"sort"
	"strings"
)

// asString 은 값이 JSON 문자열이면 그 값을 준다. null 은 문자열이 아니다 —
// encoding/json 은 null 을 string 에 넣어도 오류 없이 "" 를 주므로 먼저 막는다.
func asString(raw json.RawMessage) (string, bool) {
	if strings.TrimSpace(string(raw)) == "null" {
		return "", false
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	return v, true
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
