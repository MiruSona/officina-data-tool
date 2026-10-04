// Package migrate 는 스키마를 바꿀 때 데이터 행을 따라 고친다 (스키마·enum 편집 설계 4장).
//
// 웹이 보낸 「무엇을 바꿨나(ops)」를 옛 스키마 위에서 차례로 흉내 내 이름 지도를 만들고,
// 그 지도로 행의 열 이름·enum 값을 옮긴다. 「지우기+더하기」로 짐작하지 않는다 —
// op 없이 사라진 열·값은 막는다. HTTP 도 디스크도 모르는 순수 함수다.
package migrate

import (
	"fmt"
	"regexp"

	"github.com/mirusona/officina-data-tool/internal/schema"
	"github.com/mirusona/officina-data-tool/internal/validate"
)

// op 이름이다. 웹이 보내는 "op" 칸에 그대로 온다.
const (
	OpRenameColumn    = "renameColumn"
	OpDropColumn      = "dropColumn"
	OpRenameEnumValue = "renameEnumValue"
	OpDropEnumValue   = "dropEnumValue"
	OpRenameEnum      = "renameEnum"
)

// RuleOp 는 ops 가 틀렸거나 ops 로 못 옮기는 자리를 알리는 규칙 이름이다.
const RuleOp = "op"

// 이름 꼴은 schema 묶음과 같다. op 의 새 이름을 먼저 막아야 오류가 op 자리를 가리킨다.
var (
	reLowerName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	reEnumName  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
)

// Op 는 바꾼 것 하나다. 칸은 op 마다 쓰는 것만 채운다 (설계 3-1).
type Op struct {
	Op          string `json:"op"`
	Table       string `json:"table,omitempty"`
	Column      string `json:"column,omitempty"`
	Enum        string `json:"enum,omitempty"`
	Value       string `json:"value,omitempty"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	ReplaceWith string `json:"replaceWith,omitempty"`
}

// Plan 은 옛 이름 → 지금 이름 지도다. 지도의 키는 늘 **옛 스키마의 이름**이다.
type Plan struct {
	old *schema.File
	// 표 → 옛 열 → 지금 열. "" 면 지웠다.
	columns map[string]map[string]string
	// 옛 enum → 지금 enum 이름.
	enums map[string]string
	// 옛 enum → 옛 값 → 지금 값. 지운 값은 대체 값이거나 "" 다.
	values map[string]map[string]string
	// 옛 enum → 지운 옛 값 (대체했든 안 했든). C# 숫자 경고에서 뺀다.
	dropped map[string]map[string]bool
}

// New 는 ops 를 차례로 흉내 내 지도를 만든다. 앞 op 뒤의 이름으로 다음 op 를 읽는다.
// 틀린 op 가 있으면 문제 목록을 준다 (첫 건에서 안 멈춘다).
func New(old *schema.File, ops []Op) (*Plan, []*validate.Problem) {
	p := &Plan{
		old:     old,
		columns: map[string]map[string]string{},
		enums:   map[string]string{},
		values:  map[string]map[string]string{},
		dropped: map[string]map[string]bool{},
	}
	for _, t := range old.Tables {
		p.columns[t.Name] = map[string]string{}
		for _, c := range t.Columns {
			p.columns[t.Name][c.Name] = c.Name
		}
	}
	for name, values := range old.Enums {
		p.enums[name] = name
		p.values[name] = map[string]string{}
		p.dropped[name] = map[string]bool{}
		for _, v := range values {
			p.values[name][v] = v
		}
	}

	problems := []*validate.Problem{}
	for i, op := range ops {
		if msg := p.apply(op); msg != "" {
			problems = append(problems, opProblem(fmt.Sprintf("ops[%d] %s: %s", i, op.Op, msg)))
		}
	}
	return p, problems
}

// apply 는 op 하나를 지도에 얹는다. 틀렸으면 까닭을, 맞으면 "" 를 준다.
func (p *Plan) apply(op Op) string {
	switch op.Op {
	case OpRenameColumn:
		return p.renameColumn(op)
	case OpDropColumn:
		return p.dropColumn(op)
	case OpRenameEnum:
		return p.renameEnum(op)
	case OpRenameEnumValue:
		return p.renameEnumValue(op)
	case OpDropEnumValue:
		return p.dropEnumValue(op)
	}
	return fmt.Sprintf("모르는 op 다: %q (쓸 수 있는 것: %s, %s, %s, %s, %s)", op.Op,
		OpRenameColumn, OpDropColumn, OpRenameEnumValue, OpDropEnumValue, OpRenameEnum)
}

func (p *Plan) renameColumn(op Op) string {
	if op.Table == "" || op.From == "" || op.To == "" {
		return "table · from · to 칸이 다 있어야 한다"
	}
	cols, ok := p.columns[op.Table]
	if !ok {
		return fmt.Sprintf("저장된 스키마에 없는 표다: %q", op.Table)
	}
	if op.From == "id" {
		return "id 열은 이름을 못 바꾼다"
	}
	if !reLowerName.MatchString(op.To) {
		return fmt.Sprintf("열 이름 꼴이 틀렸다 (%s): %q", reLowerName, op.To)
	}
	old := findOld(cols, op.From)
	if old == "" {
		return fmt.Sprintf("표 %s 에 열 %q 가 없다", op.Table, op.From)
	}
	if findOld(cols, op.To) != "" {
		return fmt.Sprintf("표 %s 에 열 %q 가 이미 있다", op.Table, op.To)
	}
	cols[old] = op.To
	return ""
}

func (p *Plan) dropColumn(op Op) string {
	if op.Table == "" || op.Column == "" {
		return "table · column 칸이 다 있어야 한다"
	}
	cols, ok := p.columns[op.Table]
	if !ok {
		return fmt.Sprintf("저장된 스키마에 없는 표다: %q", op.Table)
	}
	if op.Column == "id" {
		return "id 열은 못 지운다"
	}
	old := findOld(cols, op.Column)
	if old == "" {
		return fmt.Sprintf("표 %s 에 열 %q 가 없다", op.Table, op.Column)
	}
	cols[old] = ""
	return ""
}

func (p *Plan) renameEnum(op Op) string {
	if op.From == "" || op.To == "" {
		return "from · to 칸이 다 있어야 한다"
	}
	if !reEnumName.MatchString(op.To) {
		return fmt.Sprintf("enum 이름 꼴이 틀렸다 (%s): %q", reEnumName, op.To)
	}
	old := findOld(p.enums, op.From)
	if old == "" {
		return fmt.Sprintf("저장된 스키마에 없는 enum 이다: %q", op.From)
	}
	if findOld(p.enums, op.To) != "" {
		return fmt.Sprintf("enum %q 가 이미 있다", op.To)
	}
	p.enums[old] = op.To
	return ""
}

func (p *Plan) renameEnumValue(op Op) string {
	if op.Enum == "" || op.From == "" || op.To == "" {
		return "enum · from · to 칸이 다 있어야 한다"
	}
	if !reLowerName.MatchString(op.To) {
		return fmt.Sprintf("enum 값 꼴이 틀렸다 (%s): %q", reLowerName, op.To)
	}
	old := findOld(p.enums, op.Enum)
	if old == "" {
		return fmt.Sprintf("저장된 스키마에 없는 enum 이다: %q", op.Enum)
	}
	if !p.hasLiveValue(old, op.From) {
		return fmt.Sprintf("enum %s 에 값 %q 가 없다", op.Enum, op.From)
	}
	if p.hasLiveValue(old, op.To) {
		return fmt.Sprintf("enum %s 에 값 %q 가 이미 있다 (합치려면 dropEnumValue 의 replaceWith)", op.Enum, op.To)
	}
	// 앞서 이 값으로 대체된 옛 값까지 같이 옮긴다 — 지도는 「옛 값 → 지금 값」이다.
	for v, cur := range p.values[old] {
		if cur == op.From {
			p.values[old][v] = op.To
		}
	}
	return ""
}

func (p *Plan) dropEnumValue(op Op) string {
	if op.Enum == "" || op.Value == "" {
		return "enum · value 칸이 다 있어야 한다"
	}
	if op.ReplaceWith == op.Value {
		return "replaceWith 가 지우는 값 자신이다"
	}
	if op.ReplaceWith != "" && !reLowerName.MatchString(op.ReplaceWith) {
		return fmt.Sprintf("replaceWith 꼴이 틀렸다 (%s): %q", reLowerName, op.ReplaceWith)
	}
	old := findOld(p.enums, op.Enum)
	if old == "" {
		return fmt.Sprintf("저장된 스키마에 없는 enum 이다: %q", op.Enum)
	}
	if !p.hasLiveValue(old, op.Value) {
		return fmt.Sprintf("enum %s 에 값 %q 가 없다", op.Enum, op.Value)
	}
	for v, cur := range p.values[old] {
		if cur != op.Value {
			continue
		}
		p.values[old][v] = op.ReplaceWith
		p.dropped[old][v] = true
	}
	return ""
}

// hasLiveValue 는 옛 enum 에서 지금 이름이 value 인 값이 있는지 본다.
func (p *Plan) hasLiveValue(oldEnum, value string) bool {
	for _, cur := range p.values[oldEnum] {
		if cur == value {
			return true
		}
	}
	return false
}

// findOld 는 지금 이름 cur 를 가진 옛 이름을 찾는다. 없으면 "".
func findOld(names map[string]string, cur string) string {
	for old, now := range names {
		if now == cur && now != "" {
			return old
		}
	}
	return ""
}

func opProblem(message string) *validate.Problem {
	return &validate.Problem{File: "schema.json", Rule: RuleOp, Message: message}
}
