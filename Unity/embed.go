// Package unity 는 게임이 가져가는 **손으로 쓴** C# 을 exe 안에 넣어 둔다 (설계 8장).
//
// 이 폴더의 파일은 생성물이 아니다. 그런데도 embed 하는 이유는 `gen` 이
// 네임스페이스를 치환해 생성 폴더에 **같이 써 내기** 때문이다 — 사람이 손으로 옮기고
// 손으로 치환하면 「Officina. 잔여 0」을 아무도 못 센다.
//
// 이 폴더에 Go 파일이 하나 있어도 Unity 는 `.go` 를 안 본다. 게임 프로젝트로
// 통째로 복사해도 아무 일이 없다.
package unity

import "embed"

// FS 는 GameDataLoader.cs · GameDataException.cs · Officina.Data.asmdef 셋이다.
//
//go:embed GameDataLoader.cs GameDataException.cs Officina.Data.asmdef
var FS embed.FS

// 파일 이름을 여기 한 번만 적는다. gen 이 이 이름으로 꺼내 쓴다.
const (
	LoaderFile    = "GameDataLoader.cs"
	ExceptionFile = "GameDataException.cs"
	AsmdefFile    = "Officina.Data.asmdef"
)
