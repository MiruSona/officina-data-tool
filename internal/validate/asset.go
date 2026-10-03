package validate

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/mirusona/officina-data-tool/internal/assetindex"
	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
)

// checkAsset 은 asset 원소 하나를 본다 — 타입·빈 값은 늘, 색인 검사(V10)는 색인이 있을 때만.
// 표는 연동 설계 3-3 이다.
func (c *checker) checkAsset(row *table.Row, col *schema.Column, where string, raw json.RawMessage) {
	v, ok := asString(raw)
	if !ok {
		c.addType(row, col, where, raw)
		return
	}
	if v == "" {
		if col.IsList {
			c.v10(row, where, RuleAsset, "list<asset> 안에 빈 주소가 있다 (빼거나 주소를 적는다)")
			return
		}
		if col.Required() {
			c.add(row, where, RuleRequired, "값이 비었다 (기본값이 없는 필수 열이다)")
		}
		return
	}
	if c.assets == nil {
		return
	}

	m := c.assets.Find(v)
	if len(m.Entries) == 0 {
		message := fmt.Sprintf("색인에 %q 가 없다", m.Address)
		if near := nearest(m.Address, c.addresses()); near != "" {
			message += fmt.Sprintf(" (가장 가까운 것: %q)", near)
		}
		c.v10(row, where, RuleAsset, message)
		return
	}
	if m.HasSub && !c.checkSub(row, where, m) {
		return
	}
	c.checkAssetKind(row, col, where, m)
	c.checkAssetBuilt(row, where, m)
}

// checkAssetDefaults 는 asset 열의 default 를 칸 값과 같은 규칙으로 본다. 표의 행과 상관없이 열마다 한 번.
// 자리는 schema.json · 줄 0 · 열 「이름.default」 다 (README 「asset 열」 V10 표). 오류·경고를 따로 돌려준다.
func (c *checker) checkAssetDefaults() ([]*Problem, []*Problem) {
	spot := &checker{sch: c.sch, st: c.st, ids: c.ids, assets: c.assets, assetWarn: c.assetWarn,
		t: &table.Table{Name: c.t.Name, Path: filepath.Join(filepath.Dir(c.t.Path), table.SchemaFileName)}}
	row := &table.Row{}
	for _, col := range c.st.Columns {
		if col.Base != schema.TypeAsset || col.Default == nil {
			continue
		}
		spot.checkValue(row, col, col.Name+".default", col.Default)
	}
	return spot.list, spot.warns
}

// checkSub 는 `address[이름]` 의 이름을 본다. 오류를 냈으면 거짓이다.
func (c *checker) checkSub(row *table.Row, where string, m assetindex.Match) bool {
	unknown := false
	for _, e := range m.Entries {
		if e.HasSub(m.Sub) {
			return true
		}
		if e.Sub == nil {
			unknown = true
		}
	}
	if unknown {
		c.warn(row, where, RuleAssetSubUnchecked,
			fmt.Sprintf("%q 의 하위 목록을 색인이 몰라 [%q] 를 못 봤다", m.Address, m.Sub))
		return true
	}
	c.v10(row, where, RuleAsset, fmt.Sprintf("%q 에 하위 에셋 %q 가 없다", m.Address, m.Sub))
	return false
}

// checkAssetKind 는 path 가 있는 항목만 놓고 하나라도 열 kind 와 맞으면 통과다.
// path 있는 항목이 없으면 kind 를 모르니 안 본다.
func (c *checker) checkAssetKind(row *table.Row, col *schema.Column, where string, m assetindex.Match) {
	if col.Kind == "" {
		return
	}
	var first *assetindex.Entry
	for _, e := range m.Entries {
		if e.Path == "" {
			continue
		}
		if e.Kind == col.Kind {
			return
		}
		if first == nil {
			first = e
		}
	}
	if first == nil {
		return
	}
	c.v10(row, where, RuleAssetKind,
		fmt.Sprintf("%s 가 와야 하는데 %s(%q) 이다", col.Kind, first.Kind, first.Path))
}

// v10 은 V10 오류 하나를 모은다. serve 저장 때는 경고 쪽으로 간다.
func (c *checker) v10(row *table.Row, where, rule, message string) {
	if c.assetWarn {
		c.warn(row, where, rule, message)
		return
	}
	c.add(row, where, rule, message)
}

// checkAssetBuilt 는 가리킨 항목이 하나도 빌드에 안 들어가면 경고한다.
func (c *checker) checkAssetBuilt(row *table.Row, where string, m assetindex.Match) {
	for _, e := range m.Entries {
		if e.IncludeInBuild {
			return
		}
	}
	c.warn(row, where, RuleAssetNotBuilt, fmt.Sprintf("%q 는 빌드에 안 들어가는 그룹(%q)에 있다", m.Address, m.Entries[0].Group))
}

// addresses 는 nearest 가 받는 꼴(이름 → 아무 값)로 주소를 준다. 표 하나에서 한 번만 만든다.
func (c *checker) addresses() map[string]int {
	if c.addressSet == nil {
		c.addressSet = map[string]int{}
		for _, a := range c.assets.Addresses() {
			c.addressSet[a] = 0
		}
	}
	return c.addressSet
}

// IndexWarnings 는 색인을 연 상태를 경고 줄로 바꾼다. 줄 번호 없이 색인 자리만 찍는다.
func IndexWarnings(st assetindex.Status) []*Problem {
	list := []*Problem{}
	if st.Missing {
		list = append(list, &Problem{File: st.File, Rule: RuleAssetIndex,
			Message: "색인이 없어 asset 검사를 건너뜀 — `assettool index` 를 돌려라"})
	}
	if st.Stale {
		list = append(list, &Problem{File: st.File, Rule: RuleAssetStale,
			Message: "색인이 낡았다 — Addressables 설정이 색인보다 새롭다. `assettool index` 를 다시 돌려라 (검사는 그대로 했다)"})
	}
	for _, note := range st.Notes {
		list = append(list, &Problem{File: st.File, Rule: RuleAssetIndex, Message: note})
	}
	return list
}
