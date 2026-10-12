package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mirusona/officina-data-tool/internal/assetindex"
)

// checkReferences 는 열 하나하나가 말이 되는지 본다.
// 이름이 실제로 있는 enum·표를 가리키는가, 칸을 엉뚱한 타입에 붙이지 않았는가,
// 기본값이 타입과 맞는가.
func checkReferences(c *collector, f *File) {
	for _, t := range f.Tables {
		for _, col := range t.Columns {
			checkColumnEnum(c, f, col)
			checkColumnRef(c, f, col)
			checkColumnBounds(c, col)
			checkColumnLoc(c, col)
			checkColumnKind(c, col)
			checkColumnDefault(c, f, col)
		}
	}
}

func checkColumnEnum(c *collector, f *File, col *Column) {
	if col.Base != TypeEnum {
		if col.Enum != "" {
			c.add(col.where+".enum", fmt.Sprintf("enum 열이 아닌데 enum 칸이 있다 (타입 %q)", col.Type))
		}
		return
	}
	if col.Enum == "" {
		c.add(col.where+".enum", "enum 열에는 enum 칸이 있어야 한다")
		return
	}
	if _, ok := f.Enums[col.Enum]; !ok {
		c.add(col.where+".enum", fmt.Sprintf("없는 enum 을 가리킨다: %q (있는 것: %s)",
			col.Enum, listOr(sortedKeys(f.Enums), "없다")))
	}
}

func checkColumnRef(c *collector, f *File, col *Column) {
	if col.Base != TypeRef {
		if col.Ref != "" {
			c.add(col.where+".ref", fmt.Sprintf("ref 열이 아닌데 ref 칸이 있다 (타입 %q)", col.Type))
		}
		return
	}
	if col.Ref == "" {
		c.add(col.where+".ref", "ref 열에는 ref 칸이 있어야 한다")
		return
	}
	if f.Table(col.Ref) == nil {
		c.add(col.where+".ref", fmt.Sprintf("없는 표를 가리킨다: %q (있는 것: %s)",
			col.Ref, listOr(f.TableNames(), "없다")))
	}
}

func checkColumnBounds(c *collector, col *Column) {
	if col.Min == nil && col.Max == nil {
		return
	}
	if col.Base != TypeInt && col.Base != TypeFloat {
		c.add(col.where, fmt.Sprintf("min·max 는 int·float 열에만 쓴다 (타입 %q)", col.Type))
		return
	}
	if col.Min != nil && col.Max != nil && *col.Min > *col.Max {
		c.add(col.where, fmt.Sprintf("min 이 max 보다 크다 (%g > %g)", *col.Min, *col.Max))
	}
}

func checkColumnLoc(c *collector, col *Column) {
	if col.Loc && col.Base != TypeString {
		c.add(col.where+".loc", fmt.Sprintf("loc 은 string 열에만 쓴다 (타입 %q)", col.Type))
	}
}

// checkColumnKind 는 kind 칸이 asset 열에만 있고 값이 계약의 다섯 중 하나인지 본다.
func checkColumnKind(c *collector, col *Column) {
	if col.Kind == "" {
		return
	}
	if col.Base != TypeAsset {
		c.add(col.where+".kind", fmt.Sprintf("kind 는 asset 열에만 쓴다 (타입 %q)", col.Type))
		return
	}
	if !contains(assetindex.Kinds, col.Kind) {
		c.add(col.where+".kind", fmt.Sprintf("모르는 kind 다: %q (쓸 수 있는 것: %s)",
			col.Kind, strings.Join(assetindex.Kinds, ", ")))
	}
}

func checkColumnDefault(c *collector, f *File, col *Column) {
	if col.Default == nil {
		return
	}
	where := col.where + ".default"
	// null 은 값이 아니다. encoding/json 은 null 을 어느 타입에 넣어도 오류를 안 내므로 먼저 막는다
	// — 그냥 두면 스키마 검사는 통과하고 굽는 자리에서 행마다 오류가 난다.
	if isNullValue(col.Default) {
		c.add(where, "기본값이 null 이다 (null 은 값이 아니다 — 칸을 빼거나 값을 적는다)")
		return
	}
	if !col.IsList {
		checkDefaultItem(c, where, f, col, col.Default)
		return
	}
	items, err := Items(col, col.Default)
	if err != nil {
		c.add(where, err.Error())
		return
	}
	for i, item := range items {
		checkDefaultItem(c, fmt.Sprintf("%s[%d]", where, i), f, col, item)
	}
}

// checkDefaultItem 은 default 원소 하나를 칸 값과 같은 변환기로 보고, 통과하면 min·max 도 본다.
func checkDefaultItem(c *collector, where string, f *File, col *Column, raw json.RawMessage) {
	if _, err := DecodeItem(f, col, raw); err != nil {
		c.add(where, err.Error())
		return
	}
	if !boundsChecked(col) {
		return
	}
	if err := CheckBounds(col, raw); err != nil {
		c.add(where, err.Error())
	}
}

// boundsChecked 는 default 에 min·max 를 견줄 열인지 본다. 아니면 checkColumnBounds 가 이미 알렸다.
func boundsChecked(col *Column) bool {
	if col.Base != TypeInt && col.Base != TypeFloat {
		return false
	}
	return col.Min == nil || col.Max == nil || *col.Min <= *col.Max
}

func listOr(items []string, empty string) string {
	if len(items) == 0 {
		return empty
	}
	return strings.Join(items, ", ")
}
