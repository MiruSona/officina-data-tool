package schema

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
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

func checkColumnDefault(c *collector, f *File, col *Column) {
	if col.Default == nil {
		return
	}
	where := col.where + ".default"
	if col.IsList {
		var items []json.RawMessage
		if err := json.Unmarshal(col.Default, &items); err != nil {
			c.add(where, fmt.Sprintf("%s 기본값은 배열이어야 한다", col.Type))
			return
		}
		for i, item := range items {
			checkValueType(c, fmt.Sprintf("%s[%d]", where, i), f, col, item)
		}
		return
	}
	checkValueType(c, where, f, col, col.Default)
}

// checkValueType 은 값 하나가 열의 원소 타입과 맞는지 본다.
func checkValueType(c *collector, where string, f *File, col *Column, raw json.RawMessage) {
	switch col.Base {
	case TypeInt:
		checkInt(c, where, raw)
	case TypeFloat:
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			c.add(where, "float 가 와야 한다")
		}
	case TypeBool:
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			c.add(where, "참거짓이 와야 한다")
		}
	case TypeString, TypeRef:
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			c.add(where, "문자열이 와야 한다")
		}
	case TypeEnum:
		checkEnumValue(c, where, f, col, raw)
	}
}

func checkInt(c *collector, where string, raw json.RawMessage) {
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		c.add(where, "int 가 와야 한다")
		return
	}
	if v != math.Trunc(v) {
		c.add(where, fmt.Sprintf("int 열인데 소수다: %g", v))
	}
}

func checkEnumValue(c *collector, where string, f *File, col *Column, raw json.RawMessage) {
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		c.add(where, "enum 값은 문자열이어야 한다")
		return
	}
	values, ok := f.Enums[col.Enum]
	if !ok {
		return // 없는 enum 은 checkColumnEnum 이 이미 알렸다
	}
	if !contains(values, v) {
		c.add(where, fmt.Sprintf("enum %s 에 없는 값이다: %q (있는 것: %s)",
			col.Enum, v, strings.Join(values, ", ")))
	}
}

func listOr(items []string, empty string) string {
	if len(items) == 0 {
		return empty
	}
	return strings.Join(items, ", ")
}
