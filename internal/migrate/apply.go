package migrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/table"
	"github.com/mirusona/officina-data-tool/internal/validate"
)

// Change 는 표 하나에서 바뀌는 양이다. plan 미리보기가 그대로 보여 준다.
type Change struct {
	// op 때문에 무엇이든 바뀐 행 수.
	RowsChanged int `json:"rowsChanged"`
	// 지운 열 때문에 사라지는 값 수 (행에 적혀 있던 것만 센다).
	ValuesLost int `json:"valuesLost"`
}

// Result 는 옮긴 결과다. 원래 표는 안 건드리고 새 표를 만든다.
type Result struct {
	Schema  *schema.File
	Tables  map[string]*table.Table
	Changes map[string]*Change
	// 사람에게 알릴 경고 글 (C# 숫자 바뀜 · 기본값 바뀜). 막지는 않는다.
	Notes []string
}

// PatchDefaults 는 새 스키마 본문의 enum 열 default 를 값 지도로 고친다 (설계 7장).
//
// **새 default 가 옛 default 와 글자가 같을 때만** 고친다 — 웹이 이미 고친 것은 안 건드린다.
// 이것을 schema.Parse 보다 먼저 하는 까닭 : 이름이 바뀐 옛 값이 default 에 남아 있으면 Parse 가 막는다.
// 본문이 깨졌으면 그대로 돌려준다 — 오류는 뒤의 Parse 가 자리와 함께 알린다.
func (p *Plan) PatchDefaults(raw []byte) ([]byte, []*validate.Problem) {
	problems := []*validate.Problem{}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw, problems
	}
	var tables []map[string]json.RawMessage
	if err := json.Unmarshal(root["tables"], &tables); err != nil {
		return raw, problems
	}

	patched := false
	for _, t := range tables {
		var name string
		var columns []map[string]json.RawMessage
		if json.Unmarshal(t["name"], &name) != nil || json.Unmarshal(t["columns"], &columns) != nil {
			continue
		}
		changed := false
		for _, col := range columns {
			newDef, ok := p.patchedDefault(name, col, &problems)
			if ok {
				col["default"] = newDef
				changed = true
			}
		}
		if changed {
			t["columns"] = mustMarshal(columns)
			patched = true
		}
	}
	if !patched {
		return raw, problems
	}
	root["tables"] = mustMarshal(tables)
	return mustMarshal(root), problems
}

// patchedDefault 는 열 하나의 고친 default 를 준다. 안 고치면 두 번째 값이 false.
func (p *Plan) patchedDefault(tableName string, col map[string]json.RawMessage, problems *[]*validate.Problem) (json.RawMessage, bool) {
	var colName string
	if json.Unmarshal(col["name"], &colName) != nil {
		return nil, false
	}
	oc := p.oldColumn(tableName, colName)
	if oc == nil || oc.Base != schema.TypeEnum || oc.Default == nil {
		return nil, false
	}
	newDef, ok := col["default"]
	if !ok || compact(newDef) != compact(oc.Default) {
		return nil, false
	}
	mapped, lost := p.mapEnumRaw(oc.Enum, oc.Default)
	if len(lost) > 0 {
		*problems = append(*problems, opProblem(fmt.Sprintf(
			"열 %s.%s 의 기본값이 지운 값 %q 다 — dropEnumValue 에 replaceWith 를 주거나 기본값을 바꾼다",
			tableName, colName, lost[0])))
		return nil, false
	}
	if compact(mapped) == compact(oc.Default) {
		return nil, false
	}
	return mapped, true
}

// oldColumn 은 지금 이름 cur 인 열의 옛 정의를 준다. 새로 더한 열이면 nil.
func (p *Plan) oldColumn(tableName, cur string) *schema.Column {
	st := p.old.Table(tableName)
	if st == nil {
		return nil
	}
	old := findOld(p.columns[tableName], cur)
	if old == "" {
		return nil
	}
	return st.Column(old)
}

// Apply 는 새 스키마에 맞춰 표들을 옮긴다. ops 로 못 옮기는 것이 있으면 문제 목록을 준다.
func (p *Plan) Apply(next *schema.File, tables map[string]*table.Table) (*Result, []*validate.Problem) {
	problems := p.checkCoverage(next)
	if len(problems) > 0 {
		return nil, problems
	}

	r := &Result{Schema: next, Tables: map[string]*table.Table{}, Changes: map[string]*Change{}}
	for _, name := range next.TableNames() {
		src := tables[name]
		if src == nil {
			continue // 파일 집합(V8)은 표를 읽을 때 이미 맞춰 봤다
		}
		moved, change, rowProblems := p.moveTable(name, src)
		problems = append(problems, rowProblems...)
		r.Tables[name] = moved
		r.Changes[name] = change
	}
	if len(problems) > 0 {
		return nil, problems
	}
	r.Notes = append(p.numberNotes(next), p.defaultNotes(next, r.Tables)...)
	return r, problems
}

// checkCoverage 는 옛 표·열·값이 새 스키마에 다 있는지 본다 — op 없이 사라진 것을 막는다.
func (p *Plan) checkCoverage(next *schema.File) []*validate.Problem {
	problems := []*validate.Problem{}
	oldNames := p.old.TableNames()
	newNames := next.TableNames()
	sort.Strings(oldNames)
	sort.Strings(newNames)
	if fmt.Sprint(oldNames) != fmt.Sprint(newNames) {
		return append(problems, opProblem(fmt.Sprintf(
			"표 추가·지우기·이름 바꾸기는 아직 안 된다 (2판) — 옛 표 %v, 새 표 %v", oldNames, newNames)))
	}

	for _, t := range p.old.Tables {
		nt := next.Table(t.Name)
		for _, c := range t.Columns {
			cur := p.columns[t.Name][c.Name]
			if cur != "" && nt.Column(cur) == nil {
				problems = append(problems, opProblem(fmt.Sprintf(
					"열 %s.%s 가 새 스키마에 없다 — 지우려면 dropColumn, 이름을 바꿨으면 renameColumn 을 보낸다", t.Name, cur)))
			}
		}
	}
	for _, oldEnum := range sortedKeys(p.old.Enums) {
		values, ok := next.Enums[p.enums[oldEnum]]
		if !ok {
			continue // enum 을 통째로 뺐다. 쓰던 열이 남았으면 schema.Parse 가 이미 막았다
		}
		for _, v := range p.old.Enums[oldEnum] {
			cur := p.values[oldEnum][v]
			if cur != "" && !contains(values, cur) {
				problems = append(problems, opProblem(fmt.Sprintf(
					"enum %s 의 값 %q 가 새 스키마에 없다 — 지우려면 dropEnumValue, 이름을 바꿨으면 renameEnumValue 를 보낸다",
					p.enums[oldEnum], cur)))
			}
		}
	}
	return problems
}

// moveTable 은 표 하나의 행을 새 이름으로 옮긴다. 원래 표는 안 바꾼다.
func (p *Plan) moveTable(name string, src *table.Table) (*table.Table, *Change, []*validate.Problem) {
	st := p.old.Table(name)
	out := &table.Table{Name: src.Name, Path: src.Path, Raw: src.Raw}
	change := &Change{}
	problems := []*validate.Problem{}
	for _, row := range src.Rows {
		moved := &table.Row{Line: row.Line, Values: map[string]json.RawMessage{}}
		touched := false
		for _, key := range row.Keys {
			value := row.Values[key]
			oc := st.Column(key)
			if oc == nil {
				// 스키마에 없는 열은 그대로 둔다 — validate(V2)가 잡는다.
				moved.Keys = append(moved.Keys, key)
				moved.Values[key] = value
				continue
			}
			cur := p.columns[name][key]
			if cur == "" {
				change.ValuesLost++
				touched = true
				continue
			}
			if cur != key {
				touched = true
			}
			if oc.Base == schema.TypeEnum {
				mapped, lost := p.mapEnumRaw(oc.Enum, value)
				if len(lost) > 0 {
					problems = append(problems, &validate.Problem{
						File: filepath.ToSlash(filepath.Base(src.Path)), Line: row.Line, Table: name, Row: row.ID(), Column: key,
						Rule: RuleOp, Message: fmt.Sprintf("지우는 값 %q 를 쓰는 행이다 — dropEnumValue 에 replaceWith 를 준다", lost[0])})
				}
				if compact(mapped) != compact(value) {
					touched = true
					value = mapped
				}
			}
			moved.Keys = append(moved.Keys, cur)
			moved.Values[cur] = value
		}
		if touched {
			change.RowsChanged++
		}
		out.Rows = append(out.Rows, moved)
	}
	return out, change, problems
}

// mapEnumRaw 는 enum 값(하나 또는 배열)을 값 지도로 옮긴다.
// 대체 없이 지운 값을 만나면 lost 에 담는다. 꼴이 틀린 값은 그대로 둔다 (V3·V9 몫).
func (p *Plan) mapEnumRaw(oldEnum string, raw json.RawMessage) (json.RawMessage, []string) {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		next, lost := p.mapValue(oldEnum, one)
		if lost {
			return raw, []string{one}
		}
		return mustMarshal(next), nil
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return raw, nil
	}
	lostAll := []string{}
	for i, item := range items {
		var v string
		if json.Unmarshal(item, &v) != nil {
			continue
		}
		next, lost := p.mapValue(oldEnum, v)
		if lost {
			lostAll = append(lostAll, v)
			continue
		}
		items[i] = mustMarshal(next)
	}
	return mustMarshal(items), lostAll
}

// mapValue 는 옛 값 하나를 지금 값으로 옮긴다. 대체 없이 지운 값이면 lost.
// 옛 enum 에 없던 값은 그대로 둔다 (V5 몫).
func (p *Plan) mapValue(oldEnum, value string) (string, bool) {
	cur, known := p.values[oldEnum][value]
	if !known {
		return value, false
	}
	if cur == "" {
		return value, true
	}
	return cur, false
}

// numberNotes 는 옛·새에 다 있는 값의 C# 숫자가 바뀌었는지 본다.
// 게임이 enum 을 int 로 저장했다면 그 값이 어긋난다 (결정 3의 까닭).
func (p *Plan) numberNotes(next *schema.File) []string {
	notes := []string{}
	for _, oldEnum := range sortedKeys(p.old.Enums) {
		newName := p.enums[oldEnum]
		newValues, ok := next.Enums[newName]
		if !ok {
			continue
		}
		oldNums := p.old.EnumNumbersOf(oldEnum)
		newNums := next.EnumNumbersOf(newName)
		for i, v := range p.old.Enums[oldEnum] {
			if p.dropped[oldEnum][v] {
				continue
			}
			j := indexOf(newValues, p.values[oldEnum][v])
			if j >= 0 && newNums[j] != oldNums[i] {
				notes = append(notes, fmt.Sprintf(
					"enum %s 값 %s 의 C# 숫자가 %d → %d 로 바뀐다 — 게임이 int 로 저장한 값이 있으면 어긋난다",
					newName, newValues[j], oldNums[i], newNums[j]))
			}
		}
	}
	return notes
}

// defaultNotes 는 기본값이 바뀐 열과, 값을 안 적어 따라 바뀌는 행 수를 알린다.
// 값을 행에 박지는 않는다 — 기본값은 「빈 칸의 값」이라는 뜻을 지킨다.
func (p *Plan) defaultNotes(next *schema.File, tables map[string]*table.Table) []string {
	notes := []string{}
	for _, t := range next.Tables {
		for _, c := range t.Columns {
			oc := p.oldColumn(t.Name, c.Name)
			if oc == nil || oc.Default == nil || c.Default == nil {
				continue
			}
			before := oc.Default
			if oc.Base == schema.TypeEnum {
				before, _ = p.mapEnumRaw(oc.Enum, oc.Default)
			}
			if compact(before) == compact(c.Default) {
				continue
			}
			blank := 0
			if moved := tables[t.Name]; moved != nil {
				for _, row := range moved.Rows {
					if _, ok := row.Values[c.Name]; !ok {
						blank++
					}
				}
			}
			if blank > 0 {
				notes = append(notes, fmt.Sprintf("열 %s.%s 기본값이 %s → %s 로 바뀐다 — 값을 안 적은 행 %d 개가 따라 바뀐다",
					t.Name, c.Name, compact(before), compact(c.Default), blank))
			}
		}
	}
	return notes
}

func compact(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// mustMarshal 은 이미 JSON 인 것·문자열·RawMessage 묶음만 받는다 — 실패할 수 없는 자리다.
func mustMarshal(v any) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("migrate: JSON 으로 못 적었다: %v", err))
	}
	return out
}

func contains(list []string, v string) bool {
	return indexOf(list, v) >= 0
}

func indexOf(list []string, v string) int {
	for i, one := range list {
		if one == v {
			return i
		}
	}
	return -1
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
