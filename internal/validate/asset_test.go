package validate_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusona/officina-data-tool/internal/assetindex"
	"github.com/mirusona/officina-data-tool/internal/assettest"
	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
	"github.com/mirusona/officina-data-tool/internal/validate"
)

// runAsset 은 card 표를 rows 로 갈아 끼우고 V10 까지 돌린다. withIndex 가 거짓이면 색인 없이 돈다.
func runAsset(t *testing.T, rows string, withIndex bool) ([]*validate.Problem, []*validate.Problem) {
	t.Helper()
	l := assettest.New(t)
	assettest.Write(t, filepath.Join(l.GameData, "card.json"), "[\n"+rows+"\n]\n")
	sch, err := schema.Load(filepath.Join(l.GameData, table.SchemaFileName))
	if err != nil {
		t.Fatal(err)
	}
	tables, err := table.LoadAll(l.GameData, sch)
	if err != nil {
		t.Fatal(err)
	}
	var ix *assetindex.Index
	if withIndex {
		if ix, err = assetindex.Load(l.Index); err != nil {
			t.Fatal(err)
		}
	}
	return validate.RunWithIndex(sch, tables, ix)
}

// A2. 연동 설계 3-3 의 V10 표 줄마다 하나씩. 행 하나가 줄 하나라 i번째 행은 i+1번 줄이다.
func TestAssetRules(t *testing.T) {
	cases := []struct {
		name     string
		row      string
		problems []want
		warnings []want
	}{
		{"ok", `{"id":"a","icon":"icons[icon_sword]","sfx":["Sfx/hit.wav"],"any":"Hero"}`, nil, nil},
		{"required-empty", `{"id":"a","icon":""}`, []want{{"card.json", 2, "icon", validate.RuleRequired}}, nil},
		{"optional-empty", `{"id":"a","icon":"icons","any":""}`, nil, nil},
		{"list-empty", `{"id":"a","icon":"icons","sfx":[""]}`, []want{{"card.json", 2, "sfx[0]", validate.RuleAsset}}, nil},
		{"missing", `{"id":"a","icon":"icns"}`, []want{{"card.json", 2, "icon", validate.RuleAsset}}, nil},
		{"sub-missing", `{"id":"a","icon":"icons[icon_axe]"}`, []want{{"card.json", 2, "icon", validate.RuleAsset}}, nil},
		{"sub-unchecked", `{"id":"a","icon":"icons","any":"model[Body]"}`, nil, []want{{"card.json", 2, "any", validate.RuleAssetSubUnchecked}}},
		{"kind", `{"id":"a","icon":"Hero"}`, []want{{"card.json", 2, "icon", validate.RuleAssetKind}}, nil},
		{"kind-dup-one-matches", `{"id":"a","icon":"dup"}`, nil, nil},
		{"kind-empty-path", `{"id":"a","icon":"pkg_icon"}`, nil, nil},
		{"kind-path-only", `{"id":"a","icon":"mixed"}`, []want{{"card.json", 2, "icon", validate.RuleAssetKind}}, nil},
		{"not-built", `{"id":"a","icon":"unbuilt"}`, nil, []want{{"card.json", 2, "icon", validate.RuleAssetNotBuilt}}},
		{"type", `{"id":"a","icon":3}`, []want{{"card.json", 2, "icon", validate.RuleType}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems, warnings := runAsset(t, c.row, true)
			checkProblems(t, problems, c.problems)
			checkProblems(t, warnings, c.warnings)
		})
	}
}

// 저장 때(serve)는 V10 을 처음부터 경고로 모은다. 오류 100건 자르기에 안 걸린다.
func TestAssetAsWarnings(t *testing.T) {
	l := assettest.New(t)
	rows := []string{}
	for i := 0; i < 101; i++ {
		rows = append(rows, fmt.Sprintf(`{"id":"r%d","icon":"nope%d"}`, i, i))
	}
	assettest.Write(t, filepath.Join(l.GameData, "card.json"), "[\n"+strings.Join(rows, ",\n")+"\n]\n")
	sch, err := schema.Load(filepath.Join(l.GameData, table.SchemaFileName))
	if err != nil {
		t.Fatal(err)
	}
	tables, err := table.LoadAll(l.GameData, sch)
	if err != nil {
		t.Fatal(err)
	}
	ix, err := assetindex.Load(l.Index)
	if err != nil {
		t.Fatal(err)
	}
	problems, warnings := validate.RunWith(sch, tables, validate.Options{Assets: ix, AssetsAsWarnings: true})
	if len(problems) != 0 {
		t.Fatalf("오류가 남았다:\n%s", validate.Text(problems))
	}
	if len(warnings) != validate.MaxPerTable+1 || warnings[validate.MaxPerTable].Rule != validate.RuleTooMany {
		t.Fatalf("경고 100건 + 「외 1건」 이어야 한다: %d건", len(warnings))
	}
}

// A2. 없는 주소에는 편집 거리 2 안의 가까운 주소를 붙인다. 먼 것에는 안 붙인다.
func TestAssetSuggestsNearest(t *testing.T) {
	problems, _ := runAsset(t, `{"id":"a","icon":"icns"}`+",\n"+`{"id":"b","icon":"zzzzzz"}`+",\n"+`{"id":"c","icon":"icnos[icon_sword]"}`, true)
	if len(problems) != 3 {
		t.Fatalf("문제가 셋이어야 한다:\n%s", validate.Text(problems))
	}
	if !strings.Contains(problems[0].Message, `"icons"`) {
		t.Errorf("가까운 주소를 안 보여준다: %s", problems[0].Message)
	}
	if strings.Contains(problems[1].Message, "가장 가까운") {
		t.Errorf("먼 값에 제안을 붙였다: %s", problems[1].Message)
	}
	if !strings.Contains(problems[2].Message, `"icons"`) {
		t.Errorf("하위 꼴에서도 주소 부분으로 제안해야 한다: %s", problems[2].Message)
	}
	kind, _ := runAsset(t, `{"id":"a","icon":"Hero"}`, true)
	if !strings.Contains(kind[0].Message, `image 가 와야 하는데 prefab("Assets/Prefabs/Hero.prefab")`) {
		t.Errorf("kind 오류 글이 다르다: %s", kind[0].Message)
	}
}

// 색인이 없으면(nil) V10 은 건너뛰고, 색인과 상관없는 타입·빈 값 검사만 돈다.
func TestAssetWithoutIndex(t *testing.T) {
	problems, warnings := runAsset(t, `{"id":"a","icon":"없는주소","sfx":[""]}`, false)
	checkProblems(t, problems, []want{{"card.json", 2, "sfx[0]", validate.RuleAsset}})
	checkProblems(t, warnings, nil)
}

// 색인 상태(없음·낡음·메모)는 파일 자리만 있는 경고 줄이 된다.
func TestIndexWarnings(t *testing.T) {
	got := validate.IndexWarnings(assetindex.Status{File: "x/address-index.json", Missing: true, Stale: true, Notes: []string{"메모"}})
	rules := []string{}
	for _, p := range got {
		rules = append(rules, p.Rule)
	}
	if strings.Join(rules, ",") != "asset_index,asset_stale,asset_index" {
		t.Fatalf("경고 규칙이 다르다: %v", rules)
	}
	if !strings.Contains(got[0].String(), "assettool index") {
		t.Errorf("없음 경고에 할 일이 없다: %s", got[0])
	}
}
