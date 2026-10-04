package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Format 은 스키마를 서버 정규형으로 적는다 (스키마·enum 편집 설계 2장).
//
// 사람이 diff 를 읽기 쉽게 **열 하나가 한 줄**이고, 칸 차례는 고정이며 정렬용 공백은 없다.
// 뜻이 같으면 글자도 같다 — `fmt` 와 웹 저장이 같은 파일을 낸다. 줄끝은 LF 다.
func (f *File) Format() []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  \"version\": %d,\n", f.Version)
	fmt.Fprintf(&b, "  \"namespace\": %s,\n", jsonString(f.Namespace))
	if len(f.Enums) > 0 {
		b.WriteString("  \"enums\": {\n")
		names := sortedKeys(f.Enums)
		for i, name := range names {
			fmt.Fprintf(&b, "    %s: %s", jsonString(name), f.formatEnum(name))
			b.WriteString(commaUnlessLast(i, len(names)))
		}
		b.WriteString("  },\n")
	}
	b.WriteString("  \"tables\": [\n")
	for i, t := range f.Tables {
		fmt.Fprintf(&b, "    { \"name\": %s, \"columns\": [\n", jsonString(t.Name))
		for j, col := range t.Columns {
			b.WriteString("      " + formatColumn(col))
			b.WriteString(commaUnlessLast(j, len(t.Columns)))
		}
		b.WriteString("    ] }")
		b.WriteString(commaUnlessLast(i, len(f.Tables)))
	}
	b.WriteString("  ]\n}\n")
	return b.Bytes()
}

// formatEnum 은 enum 하나를 한 줄로 적는다. 숫자가 차례 번호 그대로면 짧은 배열 꼴이다.
func (f *File) formatEnum(name string) string {
	values := f.Enums[name]
	numbers := f.EnumNumbersOf(name)
	parts := make([]string, len(values))
	if sequential(numbers) {
		for i, v := range values {
			parts[i] = jsonString(v)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	for i, v := range values {
		parts[i] = fmt.Sprintf("%s: %d", jsonString(v), numbers[i])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func sequential(numbers []int) bool {
	for i, n := range numbers {
		if n != i {
			return false
		}
	}
	return true
}

// formatColumn 은 열 하나를 한 줄로 적는다. 칸 차례 : name type enum ref kind min max default loc desc.
func formatColumn(c *Column) string {
	parts := []string{
		`"name": ` + jsonString(c.Name),
		`"type": ` + jsonString(c.Type),
	}
	if c.Enum != "" {
		parts = append(parts, `"enum": `+jsonString(c.Enum))
	}
	if c.Ref != "" {
		parts = append(parts, `"ref": `+jsonString(c.Ref))
	}
	if c.Kind != "" {
		parts = append(parts, `"kind": `+jsonString(c.Kind))
	}
	if c.Min != nil {
		parts = append(parts, `"min": `+jsonNumber(*c.Min))
	}
	if c.Max != nil {
		parts = append(parts, `"max": `+jsonNumber(*c.Max))
	}
	if c.Default != nil {
		parts = append(parts, `"default": `+compactJSON(c.Default))
	}
	if c.Loc {
		parts = append(parts, `"loc": true`)
	}
	if c.Desc != "" {
		parts = append(parts, `"desc": `+jsonString(c.Desc))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// jsonString 은 문자열을 JSON 으로 적는다. HTML 이스케이프는 끈다 — 설명의 <, > 를 사람이 읽게.
func jsonString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return `""` // 문자열은 늘 적힌다. 오면 안 되는 자리다
	}
	return strings.TrimRight(buf.String(), "\n")
}

func jsonNumber(v float64) string {
	out, err := json.Marshal(v)
	if err != nil {
		return "0" // Parse 를 지난 값은 NaN·Inf 가 아니다
	}
	return string(out)
}

func commaUnlessLast(i, n int) string {
	if i < n-1 {
		return ",\n"
	}
	return "\n"
}
