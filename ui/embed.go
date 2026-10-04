// Package ui 는 표 편집 화면을 exe 안에 넣어 둔다 (설계 9장).
//
// 파일이 여기 그대로 있는 이유는 빌드 단계를 안 두기 위해서다 —
// npm·번들러가 이 저장소에 안 들어온다. vendor/ 의 라이선스는 THIRD-PARTY.md 를 본다.
package ui

import "embed"

// FS 는 index.html · app.js · schema.js · app.css · vendor/ 를 담은 읽기 전용 파일 묶음이다.
//
//go:embed index.html app.js schema.js app.css vendor
var FS embed.FS
