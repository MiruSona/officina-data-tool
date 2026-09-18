package gen

import (
	"fmt"
	"strings"
)

// TemplateNamespace 는 `Unity/` 의 손으로 쓴 C# 이 달고 있는 네임스페이스다.
// 게임 프로젝트로 복사할 때 스키마의 namespace 로 바뀐다 (설계 8장).
const TemplateNamespace = "Officina.Data"

// ReplaceNamespace 는 손으로 쓴 Unity 파일의 네임스페이스를 갈아 끼운다.
//
// 바꾸는 것은 `Officina.Data` 하나뿐이다. 그 밖에 `Officina.` 이 남아 있으면
// 치환이 반쯤 된 것이므로 줄 번호를 붙여 오류를 낸다 — 잔여 0 을 지키는 자리다.
func ReplaceNamespace(src string, ns string) (string, error) {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		return "", fmt.Errorf("바꿔 넣을 namespace 가 비었다")
	}
	if lines := leftoverLines(src); len(lines) > 0 {
		return "", fmt.Errorf("치환할 수 없는 Officina. 이 남아 있다 : 줄 %s", joinInts(lines))
	}
	return strings.ReplaceAll(src, TemplateNamespace, ns), nil
}

// leftoverLines 는 `Officina.Data` 가 아닌 `Officina.` 이 나온 줄 번호를 준다 (1부터).
func leftoverLines(src string) []int {
	var lines []int
	for i, line := range strings.Split(src, "\n") {
		rest := line
		for {
			at := strings.Index(rest, "Officina.")
			if at < 0 {
				break
			}
			if !strings.HasPrefix(rest[at:], TemplateNamespace) {
				lines = append(lines, i+1)
				break
			}
			rest = rest[at+len(TemplateNamespace):]
		}
	}
	return lines
}

func joinInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprint(v))
	}
	return strings.Join(parts, ", ")
}
