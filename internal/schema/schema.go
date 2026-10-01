// Package schema 는 GameData/schema.json 을 읽고 구조를 검사한다.
//
// 타입 목록이 프로젝트에서 이 묶음 하나에만 있다.
// 데이터 파일(table)·검증 규칙(validate)을 알지 못한다 — 설계 2장의 경계다.
package schema

import "encoding/json"

// 열 타입 여덟 중 일곱. 나머지 하나는 list<T> 다.
const (
	TypeInt    = "int"
	TypeFloat  = "float"
	TypeBool   = "bool"
	TypeString = "string"
	TypeEnum   = "enum"
	TypeRef    = "ref"
	// TypeAsset 은 Addressables address 문자열이다. 색인 검사(V10)는 validate 가 한다.
	TypeAsset = "asset"
)

// File 은 schema.json 한 장이다.
type File struct {
	Version   int
	Namespace string
	Enums     map[string][]string
	// 적힌 차례 그대로다. 굽는 차례가 곧 이 차례다.
	Tables []*Table
}

// Table 은 표 하나의 정의다.
type Table struct {
	Name    string
	Columns []*Column
	// 오류에 찍을 자리("tables[1]"). 틀린 표가 빠지면 차례가 어긋나므로 들고 다닌다.
	where string
}

// Column 은 열 하나의 정의다. 칸은 설계 4-3 의 열이 전부다.
type Column struct {
	Name string
	// 스키마에 적힌 그대로. list<string> 이면 "list<string>" 이다.
	Type string
	// list 면 원소 타입, 아니면 Type 과 같다.
	Base   string
	IsList bool
	// 없으면 nil 이고, 그 열은 필수다 (설계 4-3).
	Default json.RawMessage
	Min     *float64
	Max     *float64
	Enum    string
	Ref     string
	Loc     bool
	Desc    string
	// asset 열이 받을 항목 종류. 비면 아무 종류나 받는다.
	Kind string
	// 오류에 찍을 자리("tables[1].columns[2]").
	where string
}

// Required 는 기본값이 없어 값이 꼭 있어야 하는 열인지 알려준다.
func (c *Column) Required() bool {
	return c.Default == nil
}

// Table 은 이름으로 표를 찾는다. 없으면 nil.
func (f *File) Table(name string) *Table {
	for _, t := range f.Tables {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// TableNames 는 적힌 차례대로 표 이름을 준다.
func (f *File) TableNames() []string {
	names := make([]string, 0, len(f.Tables))
	for _, t := range f.Tables {
		names = append(names, t.Name)
	}
	return names
}

// EnumValues 는 enum 하나의 값 목록을 준다. 없으면 두 번째 값이 false.
func (f *File) EnumValues(name string) ([]string, bool) {
	v, ok := f.Enums[name]
	return v, ok
}

// Column 은 이름으로 열을 찾는다. 없으면 nil.
func (t *Table) Column(name string) *Column {
	for _, c := range t.Columns {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ColumnIndex 는 열 차례를 준다. 이 번호가 곧 MessagePack 행 배열의 자리이고
// 생성 C# 의 [Key(n)] 이다. 없으면 -1.
func (t *Table) ColumnIndex(name string) int {
	for i, c := range t.Columns {
		if c.Name == name {
			return i
		}
	}
	return -1
}

// UsesAsset 은 asset 열이 하나라도 있는지 본다. 없으면 색인을 읽지도 않는다.
func (f *File) UsesAsset() bool {
	for _, t := range f.Tables {
		for _, c := range t.Columns {
			if c.Base == TypeAsset {
				return true
			}
		}
	}
	return false
}
