// Package gen 은 스키마에서 Unity 가 쓸 C# 코드를 만든다.
//
// 만드는 것은 세 갈래다 — enum 하나당 `<Enum>.cs`, 표 하나당 `<Table>Row.cs`,
// 그리고 통 하나 `GameDataTables.cs`.
//
// 여기에 더해 **손으로 쓴 `Unity/` 셋**(`GameDataLoader.cs`·`GameDataException.cs`·asmdef)도
// 같은 폴더에 낸다. 그것은 생성물이 아니라 exe 안에 품고 있다가 네임스페이스만 갈아 끼워
// 옮겨 적는 것이다 (설계 8장 · runtime.go). 사람이 손으로 옮기면 「Officina. 잔여 0」을 아무도 안 센다.
//
// 같은 스키마면 언제 돌려도 바이트까지 같은 결과가 나온다 — 생성 시각을 박지 않고
// 맵 순회는 전부 정렬한다 (T7).
package gen

import (
	"strings"
	"unicode"
)

// pascal 은 스키마 이름을 C# 이름으로 바꾼다.
//
// `_` 로 나눈 뒤 토막마다 첫 글자만 대문자로 올리고 나머지는 그대로 둔다.
// 그래서 `monster_id` 도 `monsterId` 도 똑같이 `MonsterId` 가 된다.
func pascal(name string) string {
	var b strings.Builder
	for _, part := range strings.Split(name, "_") {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

// className 은 표 이름에서 행 클래스 이름을 만든다. `item` → `ItemRow`.
func className(table string) string {
	return pascal(table) + "Row"
}

// enumNamesClass 는 enum 이름에서 이름표 클래스 이름을 만든다. `Grade` → `GradeNames`.
//
// 구운 파일에는 enum 이 **이름 문자열**로 들어가므로(설계 7장의 타입 표) 문자열과
// enum 을 잇는 자리가 따로 있어야 한다.
func enumNamesClass(enum string) string {
	return pascal(enum) + "Names"
}

// serializedProperty 는 MessagePack 이 실제로 읽고 쓰는 속성 이름이다.
//
// enum 열만 다르다 — 값이 문자열로 오므로 `Grade` 가 아니라 `GradeName` 이 [Key(n)] 을 받고,
// `Grade` 는 그 문자열을 옮겨 주는 [IgnoreMember] 속성이 된다.
func serializedProperty(colName string, isEnum bool, isList bool) string {
	base := pascal(colName)
	if !isEnum {
		return base
	}
	if isList {
		return base + "Names"
	}
	return base + "Name"
}
