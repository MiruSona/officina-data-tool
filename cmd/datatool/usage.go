package main

func usage() string {
	return `datatool — 게임 데이터 원본(JSON)을 검증하고 Unity 로 내보낸다.

쓰는 법 :
  datatool <명령> [--data DIR] [--json] [옵션…]

명령 :
  init        데이터 폴더에 예제 schema.json·표 두 장을 만든다 (있으면 안 덮는다)
  version     판과 빌드 시각
  fmt         데이터 JSON 을 규칙대로 다시 쓴다 [--check]
  validate    스키마와 데이터를 검사한다 (V1~V9)
  export      gamedata.bytes 를 굽는다 [--out 파일]
  gen         C# 코드를 만든다 [--out 폴더]
  serve       표 편집 UI 를 띄운다 [--port N] [--open]

전역 옵션 :
  --data DIR  데이터 폴더(GameData). 안 주면 ./GameData
  --json      결과를 JSON 한 덩어리로 낸다 (AI·스크립트용)

종료 코드 :
  0 성공 · 1 사용법 잘못 · 2 데이터 검증 실패 · 3 스키마가 틀림
  4 읽기 실패 · 5 쓰기 실패
`
}
