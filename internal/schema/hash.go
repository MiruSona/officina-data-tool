package schema

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// HashLen 은 schemaHash 의 글자 수다 (sha256 앞 16자).
const HashLen = 16

// Hash 는 스키마의 schemaHash 다.
//
// 파일 바이트가 아니라 **정규화한 내용**을 해시한다. 들여쓰기나 칸 차례를 바꿨다고
// 게임이 "스키마가 달라졌다"고 소리치면 안 되기 때문이다 (설계 7장).
func (f *File) Hash() string {
	sum := sha256.Sum256([]byte(f.canonical()))
	return hex.EncodeToString(sum[:])[:HashLen]
}

// canonical 은 스키마를 한 줄씩 늘어놓은 정규형이다. 뜻이 같으면 글자도 같다.
func (f *File) canonical() string {
	var b strings.Builder
	fmt.Fprintf(&b, "version=%d\n", f.Version)
	fmt.Fprintf(&b, "namespace=%s\n", f.Namespace)
	for _, name := range sortedKeys(f.Enums) {
		fmt.Fprintf(&b, "enum %s=%s\n", name, strings.Join(f.Enums[name], ","))
	}
	for _, t := range f.Tables {
		fmt.Fprintf(&b, "table %s\n", t.Name)
		for i, col := range t.Columns {
			fmt.Fprintf(&b, "  %d %s\n", i, canonicalColumn(col))
		}
	}
	return b.String()
}

// canonicalColumn 은 열 하나를 한 줄로 적는다.
// desc 는 설명뿐이라 뺀다 — 주석을 고쳤다고 데이터를 다시 구울 이유가 없다.
func canonicalColumn(c *Column) string {
	parts := []string{c.Name, c.Type}
	if c.Enum != "" {
		parts = append(parts, "enum="+c.Enum)
	}
	if c.Ref != "" {
		parts = append(parts, "ref="+c.Ref)
	}
	if c.Min != nil {
		parts = append(parts, fmt.Sprintf("min=%v", *c.Min))
	}
	if c.Max != nil {
		parts = append(parts, fmt.Sprintf("max=%v", *c.Max))
	}
	if c.Default != nil {
		parts = append(parts, "default="+compactJSON(c.Default))
	}
	if c.Loc {
		parts = append(parts, "loc")
	}
	return strings.Join(parts, " ")
}

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}
