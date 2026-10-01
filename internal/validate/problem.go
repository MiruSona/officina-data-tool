// Package validate 는 데이터가 스키마와 맞는지 본다 (설계 6장 V1~V9 · 연동 설계 3-3 V10).
//
// V1(JSON 이 깨졌나)과 V8(파일 이름 = 표 이름)은 여기 오기 전에 걸린다 —
// 파일을 읽는 것은 table 묶음이고, 읽지도 못한 파일은 검사할 것이 없기 때문이다.
// 이 묶음은 파일을 읽지도 쓰지도 않는다. 이미 읽어 온 것만 본다.
package validate

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// 규칙 이름이다. --json 의 "rule" 칸에 그대로 나간다 (설계 5장의 보기).
const (
	RuleUnknown  = "unknown"  // V2 스키마에 없는 열
	RuleRequired = "required" // V2 필수 열이 비었다
	RuleType     = "type"     // V3 타입이 안 맞는다
	RuleRange    = "range"    // V4 min·max 밖
	RuleEnum     = "enum"     // V5 enum 목록 밖
	RuleRef      = "ref"      // V6 가리키는 id 가 없다
	RuleDupID    = "dup_id"   // V7 같은 id 가 두 줄
	RuleIDForm   = "id_form"  // V7 id 꼴이 아니다
	RuleList     = "list"     // V9 list<T> 가 배열이 아니다
	RuleTooMany  = "too_many" // 표 하나에서 잘라낸 나머지

	RuleAsset     = "asset"      // V10 색인에 없는 주소 · 없는 하위 · list 안 빈 값
	RuleAssetKind = "asset_kind" // V10 항목 kind 가 열 kind 와 다르다
)

// V10 경고 이름이다. 경고는 오류와 따로 모이고 종료 코드를 안 바꾼다.
const (
	RuleAssetIndex        = "asset_index"         // 색인이 없다 · 낡음을 못 본다
	RuleAssetStale        = "asset_stale"         // 색인이 낡았다
	RuleAssetSubUnchecked = "asset_sub_unchecked" // 하위 목록을 몰라 못 봤다
	RuleAssetNotBuilt     = "asset_not_built"     // 빌드에 안 들어가는 항목
)

// Problem 은 데이터가 틀린 자리 하나다.
//
// 줄 번호가 곧 행 번호다 — 「행 하나가 한 줄」(설계 3장)이 여기서 값을 한다.
// Line 0 은 줄을 못 집는 것(잘라냄 알림)뿐이다.
// 칸 이름은 --json 에 그대로 나간다 (설계 5장의 보기와 같은 이름이다).
type Problem struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Table   string `json:"table"`
	Row     string `json:"row"`
	Column  string `json:"column"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// String 은 오류 한 줄이다. 꼴은 `파일:줄: 표.열 — 무엇이 잘못됐나` (설계 6장).
func (p *Problem) String() string {
	where := p.File
	if p.Line > 0 {
		where = fmt.Sprintf("%s:%d", p.File, p.Line)
	}
	what := p.Table
	if p.Column != "" {
		what = p.Table + "." + p.Column
	}
	if what == "" {
		return where + ": " + p.Message
	}
	return where + ": " + what + " — " + p.Message
}

// Lines 는 문제를 사람이 보는 줄 목록으로 만든다.
func Lines(problems []*Problem) []string {
	lines := make([]string, 0, len(problems))
	for _, p := range problems {
		lines = append(lines, p.String())
	}
	return lines
}

// Text 는 문제 전부를 여러 줄 한 덩어리로 만든다.
func Text(problems []*Problem) string {
	return strings.Join(Lines(problems), "\n")
}

// sortProblems 는 파일 → 줄 → 열 차례로 세운다.
// 사람이 편집기에서 위에서 아래로 훑으며 고치는 차례다.
func sortProblems(problems []*Problem, columnOrder map[string]int) {
	sort.SliceStable(problems, func(i, j int) bool {
		a, b := problems[i], problems[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		ai, bi := columnRank(columnOrder, a.Column), columnRank(columnOrder, b.Column)
		if ai != bi {
			return ai < bi
		}
		return a.Column < b.Column
	})
}

// columnRank 는 열 이름을 스키마 차례로 바꾼다.
// "tags[1]" 처럼 원소 자리가 붙은 것은 열 이름만 떼어 본다.
// 스키마에 없는 열은 맨 뒤다 — 아는 열부터 고치는 편이 낫다.
func columnRank(order map[string]int, column string) int {
	name := column
	if i := strings.IndexByte(name, '['); i >= 0 {
		name = name[:i]
	}
	if rank, ok := order[name]; ok {
		return rank
	}
	return 1 << 30
}

// Relativize 는 문제의 file 칸을 데이터 폴더 기준 상대경로로 바꾼다 (설계 5장의 보기 꼴).
//
// 검사 자체는 파일을 읽어 온 경로를 그대로 들고 있는다 — 그 경로가 어디서
// 불렀느냐에 따라 길어지면 오류 줄이 읽기 어려워진다. 사람에게 보여 주기 직전에 한 번 줄인다.
// 데이터 폴더 밖을 가리키는 것은 손대지 않는다.
func Relativize(problems []*Problem, dir string) {
	for _, p := range problems {
		rel, err := filepath.Rel(dir, filepath.FromSlash(p.File))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		p.File = filepath.ToSlash(rel)
	}
}
