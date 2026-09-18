# 함께 배포하는 남의 코드 (THIRD-PARTY)

`datatool.exe` 한 장에 들어가는 남의 코드는 **웹 표 UI 가 쓰는 Tabulator 하나뿐**이다.
Go 쪽 의존성은 0 개다 — `go.mod` 에 `require` 가 한 줄도 없다 (`CGO_ENABLED=0`).

- Tabulator 의 `.js`·`.css` 는 `ui/vendor/` 에 그대로 두고 `go:embed` 로 exe 안에 넣는다.
  **런타임에 CDN 을 부르지 않는다** — 오프라인에서도 표 UI 가 떠야 한다.
- npm · node_modules · 번들러는 이 저장소에 안 들어온다. 빌드 단계가 없다 (설계 9장).
- 라이선스 전문은 `ui/vendor/tabulator-LICENSE.txt` 에 같이 둔다.

## 목록

| 이름 | 판 | 라이선스 | 저작권 | 받은 곳 |
| --- | --- | --- | --- | --- |
| Tabulator (`tabulator-tables`) | 6.5.3 | MIT | Copyright (c) 2015-2026 Oli Folkerd | `https://cdn.jsdelivr.net/npm/tabulator-tables@6.5.3/dist/` |

들어 있는 파일 :

| 파일 | 무엇 |
| --- | --- |
| `ui/vendor/tabulator.min.js` | 표 그리기·편집기·되돌리기(history)·붙여넣기(clipboard) |
| `ui/vendor/tabulator.min.css` | 기본 테마. 우리 색 토큰에 맞추는 덮어쓰기는 `ui/app.css` 에 있다 |
| `ui/vendor/tabulator-LICENSE.txt` | MIT 전문 |

## 판을 올릴 때

1. `https://cdn.jsdelivr.net/npm/tabulator-tables@<판>/dist/js/tabulator.min.js` 와 `…/css/tabulator.min.css`,
   그리고 `…@<판>/LICENSE` 를 받아 `ui/vendor/` 의 같은 이름으로 덮는다.
2. 위 표의 판과 URL 을 고친다.
3. `datatool serve` 를 띄워 **표가 그려지는지 · enum 드롭다운이 뜨는지 · 붙여넣기가 되는지**를 눈으로 본다
   (설계 9장 「1차에서 눈으로 볼 넷」).
