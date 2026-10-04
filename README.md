# DataTool — 게임 데이터 툴

게임 데이터 원본(JSON)을 **검증하고 Unity 로 내보내는** 단일 실행 파일이다.
원본은 사람과 git 이 읽는 JSON, 런타임은 MessagePack 한 장(`gamedata.bytes`)이다.

- Go 표준 라이브러리만 쓴다 (바깥 라이브러리 0개 · `CGO_ENABLED=0`). 상주 프로세스가 없다.
- **원본 JSON 을 고치는 명령은 `fmt` 와 `serve` 둘뿐이다.** 나머지는 읽기만 하거나 제 산출물 폴더만 쓴다.
- **`--force` 같은 우회 옵션이 없다.** 검증을 억지로 통과시키는 문을 열면 그 문이 기본값이 된다.

## 흐름

```
GameData/schema.json + item.json · monster.json · drop.json
        │
        ├─ validate ──→ 걸린 것을 파일·줄·규칙 이름으로 알린다 (종료 2·3)
        │
        ├─ export ───→ Assets/StreamingAssets/gamedata.bytes   (16바이트 머리 + MessagePack)
        └─ gen ──────→ Assets/_Project/Scripts/Data/Generated/*.cs
                                     │
                                     └─→ Unity 가 GameDataLoader 로 읽는다
```

`export` 와 `gen` 은 **먼저 validate 를 돌린다.** 걸리면 한 바이트도 안 쓴다.

## 빌드

```powershell
.\build.ps1          # bin\datatool.exe
.\build.ps1 -Test    # go vet · go test 까지 돌리고 빌드
```

Go 1.26 이상이 필요하다. DataTool 폴더 안에서 친다.

**소스를 받은 뒤에는(서브모듈 갱신 포함) `.\build.ps1` 로 다시 빌드한다.**
`bin` 은 git 에 안 올라가서, 소스만 새것이고 실행 파일은 옛 판으로 남는다.
지금 실행 파일이 어느 판인지는 `datatool version` 이 알려준다 (커밋·빌드 시각).
커밋 뒤에 소스를 손댄 채로 빌드했으면 커밋 뒤에 `-dirty` 가 붙는다.

## 빨리 써 보기

```powershell
mkdir demo; cd demo
..\bin\datatool.exe init          # GameData/ 에 예제 한 벌
..\bin\datatool.exe validate      # OK 2 tables, 4 rows
..\bin\datatool.exe export        # gamedata.bytes 를 굽는다
..\bin\datatool.exe gen           # C# 을 만든다
```

## 명령

```
datatool <명령> [--data DIR] [--json] [옵션…]
```

| 명령 | 하는 일 | 쓰나 |
| --- | --- | --- |
| `init` | 데이터 폴더에 예제 `schema.json` · `.datatool.json` · 표 두 장을 만든다 | **쓴다** — 한 장이라도 이미 있으면 아무것도 안 쓰고 종료 1 |
| `fmt` | 데이터 JSON 을 규칙대로 다시 쓴다 (열 차례 · 한 줄 한 행 · 기본값 빼기). `schema.json` 도 정규형(아래 「스키마 파일 꼴」)으로 다시 쓴다 — 뜻·해시는 그대로다. **스키마에 없는 열이 있으면 한 글자도 안 쓰고 종료 2** — 다시 쓰면 그 값이 사라지기 때문이다. `--check` 는 한 글자도 안 쓰고 바뀔 파일만 알린다(있으면 종료 2) | **쓴다** — 데이터 JSON · `schema.json` |
| `validate` | 스키마와 데이터를 검사한다 (V1~V10). 첫 건에서 안 멈추고 다 모아서 낸다 | 안 쓴다 |
| `export` | `gamedata.bytes` 를 굽는다. **먼저 validate** | **쓴다** — `--out` 자리 하나 |
| `gen` | C# 을 만든다 (`<표>Row.cs` · `<enum>.cs` · `GameDataTables.cs`) **+ `Unity/` 의 로더 셋을 네임스페이스 치환해 같이 낸다**. **먼저 validate** | **쓴다** — 생성 폴더 안만 |
| `serve` | 표 편집 UI 를 `127.0.0.1` 에만 띄운다. 저장은 행 배열을 엔진에 넘겨 검증 뒤 파일을 다시 쓴다 | **쓴다** — 아래 「파일을 쓰는 곳」 셋뿐 |
| `version` | 판·빌드한 커밋·빌드 시각 | 안 쓴다 |

전역 옵션은 둘뿐이다.

- `--data DIR` — 데이터 폴더(`GameData`).
- `--json` — 결과를 JSON 한 덩어리로 낸다 (AI·스크립트가 부르기 좋게). 모든 명령이 받는다.

`validate` · `export` · `gen` 은 `--require-asset-index` 도 받는다 — asset 열이 있는데 색인이 없으면 종료 4 (CI 용, 아래 「asset 열」).

### 데이터 폴더를 어떻게 찾나

`--data` 를 안 주면 **지금 폴더에서 위로 올라가며 `.datatool.json` 을 찾는다.**
한 칸마다 그 폴더 자신과 그 아래 `GameData/` 둘을 본다 — 설정은 `GameData/` 안에 살기 때문이다.
못 찾으면 폴더를 지어내지 않고 **종료 1 과 안내**다.

`.datatool.json` 은 칸이 셋이고 셋 다 **데이터 폴더 기준 상대경로**다.

```json
{
  "gen": "../Assets/_Project/Scripts/Data/Generated",
  "export": "../Assets/StreamingAssets/gamedata.bytes",
  "assetIndex": "../Library/AssetTool/address-index.json"
}
```

- `gen` · `export` 값이 두 명령의 기본 `--out` 이다. **명령에 `--out` 을 주면 그것이 이긴다.**
- `assetIndex` 는 AssetTool 이 만든 주소 색인 자리다. **안 적으면 위 값이 기본**이다 (데이터 폴더가 Unity 뿌리 바로 아래라는 가정).
  절대 경로면 stderr 에 「색인을 읽는다: <경로>」 를 한 줄 찍는다.
- **옛 exe 는 `assetIndex` 칸이 적힌 설정을 못 읽는다**(모르는 칸 → 종료 4). 칸을 적었으면 DataTool 을 새로 빌드한다.
- 절대 경로·다른 드라이브·UNC 를 적어도 막지는 않지만 **무엇을 쓸지 stderr 에 한 줄 찍는다.**
- 모르는 칸이 있으면 조용히 넘기지 않고 **종료 4** 다. 오타가 기본값으로 둔갑하지 않게.
- BOM 이 붙어 있어도 읽는다 (Windows 편집기·PowerShell 이 붙여서 저장한다).

### 표 편집 UI (`serve`)

```powershell
datatool serve --data GameData --port 0 --open
```

- **`127.0.0.1` 에만 붙고**, 뜰 때 뽑은 토큰이 붙은 주소를 한 줄 찍는다 (`--json` 이면 `{"url":…}`).
  API 는 그 토큰이 있어야 답한다 — 같은 기계의 딴 프로그램을 막는다.
- `--port` 를 안 주면 **빈 포트**를 받는다. `--open` 을 주면 기본 브라우저를 연다(못 열어도 서버는 산다).
- 화면은 exe 안에 들어 있다 (`ui/`, `go:embed`). **인터넷이 없어도 뜬다** — CDN 을 안 부른다.
  쓰는 남의 코드는 Tabulator 하나뿐이다 (`THIRD-PARTY.md`).
- **저장은 UI 가 JSON 을 만들지 않는다.** 행 배열을 엔진에 넘기면 엔진이 `validate` 를 돌리고,
  걸리면 **한 글자도 안 쓰고** 문제 목록을 돌려준다. 통과하면 `fmt` 와 **똑같은 규칙**으로 다시 쓴다 —
  그래서 UI 로 저장해도 git diff 에 고친 줄만 나온다.

| API | 하는 일 |
| --- | --- |
| `GET /api/schema` | 스키마 전체 (UI 가 이것으로 열과 편집기를 만든다) + `rev`(schema.json 바이트의 sha256) · `enumNumbers`(enum 값마다 C# 숫자) · `source`(파일 꼴 그대로) |
| `POST /api/schema/plan` | 본문 `{schema, ops}` 를 끝까지 돌려 보고 바뀔 파일·행 수·잃는 값·문제·경고(`notes`)를 준다. **한 글자도 안 쓴다** |
| `PUT /api/schema` | 같은 본문 + `If-Match: <rev>`. 통과하면 표들과 schema.json 을 쓴다. rev 가 다르면 409 · 없으면 428 · 걸리면 400 (안 씀) |
| `POST /api/gen` | CLI `gen` 과 같다 — 먼저 validate(V10 은 오류), 그다음 `.datatool.json` 의 `gen` 폴더에 C# 을 쓴다. 칸이 없으면 400 |
| `GET /api/tables` | 표 이름과 행 수 |
| `GET /api/table/{이름}` | 행 배열. 기본값이라 파일에서 빠진 열은 서버가 채워서 준다 |
| `PUT /api/table/{이름}` | 행 배열 통째 → 검증 → 통과하면 tmp→이름바꾸기로 저장 (걸리면 400) |
| `POST /api/validate` | `validate` 와 같은 일. 한 글자도 안 쓴다. V10 도 오류로 센다 · 응답에 `warnings` |
| `GET /api/assetindex` | 색인 요약 (`ok` · `missing` · `stale` · `notes` · `entries`). **부를 때마다 파일을 다시 읽는다.** 색인이 깨졌으면 200 + `ok:false` |
| `GET /api/asset?address=<주소>` | 원본 파일 바이트 (미리보기용). 막기와 형식 표는 아래 「asset 열」 |

**파일을 쓰는 곳은 셋이다.** 셋 다 토큰·Host 문지기(`guard`)와 본문 상한 32MB 를 지난다. 브라우저에서 경로를 받지 않는다.

| 길 | 쓰는 파일 | 자리 |
| --- | --- | --- |
| `PUT /api/table/{이름}` | 표 한 장 | 데이터 폴더(뜰 때 링크까지 풀어 쥔 뿌리) 안. 이름 꼴 + 뿌리 안 검사 |
| `PUT /api/schema` | 바뀌는 표 여러 장 + `schema.json` | 같은 뿌리. **tmp 를 다 쓴 뒤 rename** (표 먼저, schema.json 마지막). 링크·보통 파일 아닌 자리는 안 쓴다 |
| `POST /api/gen` | 생성 `.cs` · asmdef | `.datatool.json` 의 `gen` 칸 폴더 (뜰 때 한 번 정규화) |

**스키마·enum 편집** — 웹이 바뀐 스키마 전체와 바꾼 것 목록(`ops` : `renameColumn` · `dropColumn` · `renameEnumValue` · `dropEnumValue` · `renameEnum`)을 보내면
서버가 데이터 행까지 따라 고친다. op 없이 사라진 열·값은 막는다(짐작하지 않는다). 쓰는 행이 있는 enum 값을 지우려면 `replaceWith` 가 필요하다.
백업 폴더는 안 만든다 — plan 이 git 상태(git 아래가 아님 · 커밋 안 한 변경)를 `notes` 로 알린다. 계약 전부는 `Docs/Design/2026-10-04-스키마Enum편집설계.md`.

**「스키마」·「Enum」 탭** (위 막대의 「표 · 스키마 · Enum」)

- 「스키마」 탭 : 표를 고르면 열 목록이 뜬다 — 이름 · 형 · enum · ref · kind · min · max · default · loc · 설명. 열 추가(이름 + 형) · 이름 바꾸기 · 지우기 · ↑↓ 차례.
  `id` 열은 못 고친다. default 는 JSON 으로 적는다(따옴표 없이 적으면 글자, 빈 칸 = 필수 열). 새 열은 형에 맞는 기본값(`""` · `0` · `false` · `[]`)으로 붙는다. 표 추가·지우기는 2판이다.
- 「Enum」 탭 : enum 을 고르면 값과 C# 숫자가 뜬다. 값 추가(숫자 = 지금까지 저장된 최댓값과 지금 값 중 큰 것 + 1 — 지운 숫자는 다시 안 쓴다) · 이름 바꾸기 · 지우기 · ↑↓ 차례 · enum 추가 · enum 이름 바꾸기. 저장된 값을 지우면 「쓰던 행은 →」 대체 값을 고르는 창이 뜬다.
- 고치는 동안 데이터 행을 따라 고쳐야 하는 일(열 이름·지우기, enum 이름, 값 이름·지우기)만 `ops` 로 차례대로 쌓인다. 저장 안 한 새 열·값의 편집은 op 를 안 낸다.
- **저장은 늘 「미리보기 · 저장」 을 거친다.** 창에 바꿀 것 · 바뀔 파일(바뀌는 행 · 잃는 값) · 문제 · 경고 · 알림이 뜨고, 문제가 없을 때만 「저장」(PUT + `If-Match`)이 켜진다. 목록은 30줄까지 보이고 나머지는 「그 밖에 N건」 이다. 스키마·Enum 탭의 Ctrl+S 도 미리보기를 연다.
  409 면 「디스크가 바뀌었다 — 다시 읽기」 를, 500(쓰다 멈춤)이면 쓴 파일·못 쓴 파일을 그대로 보인다. 저장되면 표 화면도 새 스키마로 다시 연다.
- **두 저장이 서로 덮지 않게 막는다.** 저장 안 한 행 편집이 있으면 스키마·Enum 탭이 잠기고, 저장 안 한 스키마 변경이 있으면 표가 잠긴다(행 추가·지우기·저장 꺼짐). 막힌 까닭은 화면 위 빨간 띠에 한 줄로 뜬다. 「되돌리기」 는 고친 것을 버리고 디스크 것을 다시 읽는다.
- 「C# 만들기」(위 막대, 모든 탭) : `POST /api/gen`. **디스크의** 스키마·데이터로 만든다 — 저장 안 한 편집은 안 들어간다(창에 그렇게 뜬다). 쓴 파일 · 스키마에 없는 옛 파일(`stale`, 안 지운다)을 보인다. `.datatool.json` 에 `gen` 칸이 없으면 무엇을 적을지 알려 준다.
- 서버에서 온 글(문제·알림·파일 이름)은 전부 글자로만 넣는다 (U6 X2).

- **asset 칸은 저장 때 막지 않는다.** 없는 주소·틀린 종류(V10)는 `PUT` 응답의 `warnings` 로 오고 파일은 저장된다 —
  색인이 낡았을 때 새 주소를 먼저 적을 수 있어야 해서다. **오류로 막는 것은 CLI `validate` 뿐이다.**
- 화면 : image 칸은 32px 그림(하위 에셋은 그 칸만 잘라), audio 칸은 ▶ 버튼(한 번에 한 소리), 나머지는 글자 + 종류 표.
  편집은 열 `kind` 로 거른 주소 드롭다운이고 직접 써도 된다. `list<asset>` 칸은 앞에서부터 미리보기 되는 원소의 그림·▶ 를 3개까지 나란히 보이고, 안 보인 원소 전부(미리보기 없는 원소 포함)를 「+N」 으로 센 뒤 칸 글자를 둔다.
  드롭다운(enum·ref·asset) 줄은 글자로만 넣는다 — 주소·값에 든 태그가 실행되지 않는다(U6 X1). 편집은 글자 칸이다(쉼표로 나눠 쓴다).
  그림 썸네일을 누르면 가운데에 크게 뜬다 — 정수 배로 키워 픽셀이 안 뭉개지고 화면 80% 를 안 넘는다. 하위 에셋은 그 칸만, 아래에 주소·원본 크기. Esc · 덮개 · 「닫기」(Enter·Space) 로 닫고, 열린 동안 표 키는 안 먹는다.
  스키마 `default` 에 걸린 문제(줄 0, 열 `이름.default`)는 문제 목록에 `schema.json — 표.열.default` 로 뜨고, 누르면 「스키마에서 고친다」 알림만 뜬다.
  아틀라스 스프라이트(`아틀라스[이름]`)는 원본 그림에서 rect 만 잘라 보이고(w·h 0 이면 그림 전체) 종류 표는 `sprite` 다. 맨 아틀라스 주소는 첫 스프라이트 + 「N장」 표다. 색인에 없는 하위 이름은 표 없이 빨간 글자다. image 열 드롭다운에는 아틀라스의 `주소[이름]` 만 뜨고 맨 주소는 안 뜬다.
  **아틀라스 격자 창** — 맨 아틀라스 칸의 썸네일·「N장」 을 누르면 그 아틀라스의 스프라이트 전부가 타일(그림 + 이름)로 뜬다. 스프라이트 칸의 크게 보기에서는 「아틀라스 전부 보기」 로 넘어간다(그 스프라이트가 골라진 채). 타일을 누르면 고르기만 하고 아래 줄에 「이름 · 가로×세로」 가 뜬다. 「이 칸에 넣기」(또는 타일 두 번 누르기)를 눌러야 칸 값이 `주소[이름]` 으로 바뀐다 — 손으로 고친 것과 같이 「저장 안 한 변경」·검증을 탄다. `list<asset>` 칸에서 연 격자는 보기만 한다. 하위 목록을 모르거나 0장이면 격자는 안 뜬다.
  색인이 없거나 낡았거나 깨졌으면 표 위에 한 줄 띠가 뜬다.
- 붙여넣기는 CRLF·CR 을 LF 로 고르고 끝 줄바꿈 하나만 뗀다(엑셀 복사 그대로 붙여도 된다). 표 끝을 넘친 행은 **안 늘리고 빨간 알림**만 띄운다 — 행 추가 뒤 다시 붙인다.
- 편집 칸을 연 채 Ctrl+S 를 눌러도 입력을 확정한 뒤 저장한다. 서버가 꺼져 저장을 못 하면 빨간 알림이 뜨고 고친 것은 화면에 남는다.
- 「행 지우기」는 범위로 고른 행을 전부 지운다. 열 전체를 골랐거나 20행을 넘으면 확인창이 먼저 뜬다. Ctrl+Z 한 번이면 묶음째 되살아난다 — 표 밖(단추를 누른 뒤)에서도 듣는다.
- **U6(2,000행 편집 뒤 diff 가 고친 줄만인가)은 `Test/u6/run.ps1` 이 크롬 headless 로 판정한다** — 시나리오 36개(S10 은 list 원소 문제의 빨간 칸·문제 줄 이동, Z1 은 크게 보기, L1 은 「+N」, X1 은 드롭다운 XSS, G1 은 아틀라스 격자 창, A1 은 아틀라스 스프라이트, K1~K6 은 스키마·Enum 탭 — 열 이름 바꾸기 저장 · enum 값 이름 바꾸기 · 대체 값 없는 값 지우기 막힘 · 409 · 서로 잠그기 · C# 만들기, X2 는 미리보기 창 XSS) · Node 22 이상과 크롬(또는 Edge)만 있으면 된다 · 종료 0 통과 · 1 실패 · 2 환경 문제 · `-DataTool <exe>` 를 안 주면 소스로 임시 폴더에 새로 굽는다(`bin/` 은 안 건드린다).

## 종료 코드

| 코드 | 뜻 |
| --- | --- |
| 0 / 1 | 성공 / 사용법 잘못 (모르는 인자 · 데이터 폴더를 못 찾음 · `init` 이 덮으려 함) |
| **2** | **데이터 검증 실패** (타입 · 필수 · 참조 · 중복 · 범위 · asset 주소 · `fmt --check` 가 바뀔 파일을 찾음) |
| **3** | **스키마 자체가 틀림** (모르는 타입, 없는 enum, 첫 열이 `id` 가 아님, asset 이 아닌 열에 `kind`) |
| 4 / 5 | 읽기 실패 (없음 · JSON 깨짐 · 설정에 모르는 칸 · 주소 색인이 깨짐) / 쓰기 실패 |

**경고는 종료 코드를 안 바꾼다.** 텍스트는 stderr 에 `경고: …` 줄, `--json` 은 `"warnings": [...]` (문제와 같은 꼴) 이다.

2 와 3 을 가르는 이유는 **고칠 파일이 다르기** 때문이다. 3 이면 데이터를 아무리 봐도 소용없다.

## 데이터 꼴

```json
[
{"id":"sword_iron","name":"철검","atk":12,"price":300,"tags":["weapon","melee"]},
{"id":"potion_hp","name":"체력 물약","price":50,"tags":["consume"]}
]
```

1. **파일 전체가 유효한 JSON 배열**이고 **행 하나가 한 줄**이다 (`[` 와 `]` 만 자기 줄에 선다).
   git diff 가 「몇 번 아이템이 어떻게 바뀌었나」로 읽힌다.
2. **열 차례는 스키마 차례를 따르고, 기본값과 같은 열은 뺀다** (위 `potion_hp` 의 `atk`).
3. **`id` 는 문자열**이고 표 안에서 유일하다 (`^[a-z][a-z0-9_]*$`). 파일 이름 = 표 이름이다.
4. **데이터 파일은 LF 로 통일한다.** `fmt` 는 CRLF 를 LF 로 바꾼다. BOM 이 붙어 있어도 읽는다.
5. **숫자는 JS `String(Number(x))` 꼴로 적는다** — `300.0` → `300`, `1.25e1` → `12.5`, `1e21` → `1e+21`, `-0` → `0`. 열 타입은 안 본다(int 열 `1.0` 도 `1`). 웹 UI 가 보내는 꼴과 글자까지 같아서 UI 로 저장해도 안 고친 줄이 diff 에 안 나온다. float64 를 넘는 수(`1e999`)는 `fmt` 가 파일:줄을 알리고 멈춘다.

## 스키마 타입

`GameData/schema.json` 한 파일이 표 전부를 담는다. **첫 열은 반드시 `id`, 타입 `string`** 이다.
열 칸은 `name` · `type` · `default` · `min` · `max` · `enum` · `ref` · `loc` · `desc` · `kind` 열이 전부다.

| 타입 | JSON | C# | 비고 |
| --- | --- | --- | --- |
| `int` · `float` · `bool` | 숫자 · 참거짓 | `int` · `float` · `bool` | `float` 은 굽을 때 float32 |
| `string` | 문자열 | `string` | `"loc": true` 를 달 수 있다 (1차는 무시 — 2차 다국어 표시) |
| `enum` | 목록 안의 문자열 | 생성된 `enum` | `enum` 칸 필수 |
| `ref` | 다른 표의 `id` | `string` | 로더가 딕셔너리로 이어 준다 |
| `asset` | Addressables 주소 문자열 | `string` | `kind` 로 종류를 좁힐 수 있다. 하위 에셋은 `주소[이름]` (아래 「asset 열」) |
| `list<T>` | 배열 | `T[]` | `T` 는 위 일곱 중 하나. **중첩 `list` 는 안 된다** |

**enum 꼴은 둘이다.** `"Grade": ["common", "rare", "epic"]` 은 C# 숫자가 차례 번호(0,1,2)다.
`"Grade": {"common": 0, "epic": 5}` 처럼 적으면 값마다 숫자를 박는다 — 값을 지우거나 차례를 바꿔도 남은 값의 숫자가 안 밀린다
(게임이 enum 을 int 로 저장했을 때 안 어긋나게). 숫자는 0~2147483647 정수, 중복 금지. **숫자는 `schemaHash` 에 안 들어간다** — 구운 파일에는 이름 문자열만 들어간다.

**스키마 파일 꼴** — `fmt` 와 웹 저장이 같은 꼴로 적는다 : 열 하나가 한 줄, 칸 차례 `name type enum ref kind min max default loc desc`,
정렬용 공백 없음, enum 이름은 가나다 차례(값 차례는 그대로), 숫자가 차례 번호뿐인 enum 은 배열 꼴, LF.

`default` 가 없고 값도 없으면 **필수 열**이다. `required` 칸을 따로 안 만든다.
날짜·시간·벡터·중첩 문서·수식은 없다 — 필요하면 **표를 하나 더 만든다**로 푼다.

## asset 열 (AssetTool 주소 색인)

```json
{ "name": "icon", "type": "asset", "kind": "image", "default": "" },
{ "name": "sfx",  "type": "list<asset>", "kind": "audio" }
```

- 칸 값은 Addressables 주소다. 주소가 맞는지는 **AssetTool 이 만든 `address-index.json`** 으로 본다 (계약은 아래 「경계 계약」).
- `kind` 는 `prefab` · `image` · `audio` · `scene` · `other` 중 하나다. asset 이 아닌 열에 달거나 모르는 값이면 종료 3.
  없으면 아무 종류나 받는다. **`kind` 는 `schemaHash` 에 안 들어간다**(런타임 꼴이 안 바뀐다).
- C# 은 `string`(`list<asset>` 은 `string[]`), 굽기도 문자열이다.
- **스키마에 asset 열이 없으면 색인을 읽지도 않는다.**

**검증 V10**

| 경우 | CLI (`validate` · `export` · `gen`) | 규칙 이름 |
| --- | --- | --- |
| 색인 파일이 없다 | **경고** 한 줄, V10 건너뜀. `--require-asset-index` 면 **종료 4** | `asset_index` (경고) |
| 색인이 깨졌다 · 모르는 `version` · 필수 칸 없음 · 모르는 `kind` · `settingsPath` 가 `Assets/`·`Packages/` 아래 `.asset` 이 아니다 | **종료 4** | — |
| 색인이 낡았다 | 경고 한 줄, 검사는 그대로 | `asset_stale` (경고) |
| 색인의 `unityRoot` 가 데이터 폴더의 부모가 아니다 | 경고 (낡음을 못 본다), 검사는 그대로 | `asset_index` (경고) |
| 값이 `""` | 필수 열이면 `required` 오류, 선택 열이면 통과. `list<asset>` 안의 `""` 는 오류 | `required` · `asset` |
| 주소가 색인에 없다 | 오류 + 편집 거리 2 안의 가까운 주소 | `asset` |
| `주소[이름]` 인데 그 이름이 `sub` 에 없다 | 오류 + 편집 거리 2 안의 가까운 하위 이름 | `asset` |
| `주소[이름]` 인데 항목에 `sub` 가 없다 | 경고로 통과 (하위를 모른다) | `asset_sub_unchecked` (경고) |
| 항목 `kind` ≠ 열 `kind` | 오류. 같은 주소의 항목 중 **`path` 가 있는 것만** 놓고 하나라도 맞으면 통과. `path` 있는 항목이 하나도 없으면 kind 검사를 건너뛴다 | `asset_kind` |
| 아틀라스의 `주소[이름]` (이름이 `sub` 에 있거나 `sub` 가 없다) | **image 로 본다** — image 열에서 통과 | — |
| 맨 아틀라스 주소를 image 열에 | 오류 (아틀라스는 `other`) + 「스프라이트를 쓰려면 `주소[이름]` 꼴」 귀띔 | `asset_kind` |
| 가리킨 항목이 모두 `includeInBuild:false` | 경고 | `asset_not_built` (경고) |

**아틀라스** 는 `path` 가 `.spriteatlas` · `.spriteatlasv2` 로 끝나는 `other` 항목이다 (새 kind 가 없다, 확장자로 가린다). 위 표는 `default` 와 `list<asset>` 원소에도 똑같다.

`""` 검사는 색인이 없어도 돈다 — 색인이 필요 없는 검사이기 때문이다.
serve 저장 때는 V10 오류를 처음부터 경고로 모은다. 오류와 경고는 표당 100건을 **따로** 자르므로, 틀린 주소가 많아도 저장이 막히지 않는다.
**asset 열의 `default` 도 칸 값과 같은 표로 본다.** 문제 자리는 `schema.json`(줄 없음) · 열 `이름.default`(`list<asset>` 은 `이름.default[i]`)이고, 행 수와 상관없이 열마다 한 번만 낸다. 표의 칸 문제보다 앞에 서므로 표당 100건 자르기에 안 잘린다.
`""`·`[]` 기본값은 그대로 통과하고, `list<asset>` 기본값 안의 `""` 는 칸 값처럼 오류다(색인 없이도).

**`/api/asset` 막기** — 차례대로 다 통과해야 연다. 하나라도 걸리면 파일을 한 바이트도 안 준다.

1. 토큰 확인. 없으면 401.
2. **주소만 받는다**(경로 인자 없음). 색인에 없는 주소 → 404. `path` 가 빈 항목 → `preview:false`.
   같은 주소가 여럿이면 미리보기가 되고 경로가 바르고 (하위면) 그 이름을 가진 첫 항목을 연다.
   하위 목록을 아는 항목 어디에도 그 이름이 없으면 404. 아틀라스 sub 에 `path` 가 있으면 **그 파일**이 열 대상이다 —
   항목 `path` 와 sub `path` 를 여기서 하나로 모은 뒤 3~7 을 똑같이 건다.
3. **감옥 뿌리는 DataTool 이 정한다.** 색인의 `unityRoot` 를 푼 폴더가 데이터 폴더의 부모와 같을 때만 연다. 다르면 403.
4. 열 경로(항목 `path` 또는 sub `path`)는 정리된 `/` 상대경로이고 `Assets/` · `Packages/` 로 시작해야 한다. 아니면 403.
5. `os.Root` 로 연다. `../` 와 뿌리 밖을 가리키는 링크·**Windows junction** 은 `os.Root` 가 막는다 (시험 `TestOSRootBlocksJunction` 으로 확인). 파일이 없으면 404.
6. 파일이 아니거나 50MB 를 넘으면 `preview:false`.
7. Content-Type 은 확장자 표로 박고 `X-Content-Type-Options: nosniff` 를 단다. 표에 없는 확장자는 **파일을 열기 전에** `preview:false` 다 (파일이 없어도 같다).

| 브라우저가 여는 것 | 못 여는 것 → 200 + `{"preview":false,"path":…,"reason":…}` |
| --- | --- |
| png · jpg · jpeg · gif · bmp · wav · mp3 · ogg | psd · tga · tif · exr · aif · flac 등 그 밖 전부 |

## Unity 에 붙이기

`Unity/` 의 손으로 쓴 `.cs` 둘과 `.asmdef` 는 **`gen` 이 exe 안에서 꺼내 네임스페이스를 갈아 끼워
생성 폴더에 같이 써 낸다.** 사람이 복사하고 사람이 치환하면 「`Officina.` 잔여 0」을 아무도 안 세기 때문이다.
MessagePack-CSharp 설치, 첫 호출, 확인 차례(U1~U5)는 **`Unity/README.md`** 를 본다.
U1~U5 · U4b 는 **`Unity/Verify/verify.ps1 -Project <Unity 프로젝트>`** 가 배치모드로 판정한다 (종료 0 통과 · 1 실패 · 2 환경 문제).

## 폴더

| 자리 | 무엇 |
| --- | --- |
| `cmd/datatool/` | 인자 가르기와 결과 찍기만 한다 |
| `internal/schema/` | `schema.json` 읽기·검사·`schemaHash`. **타입 목록이 여기 하나뿐이다** |
| `internal/table/` | 데이터 JSON 읽기·쓰기. **「한 줄 한 행」을 아는 유일한 곳** |
| `internal/textfile/` | 읽기 입구 셋(표·스키마·설정)이 같이 쓰는 파일 읽기. **BOM 걷기가 여기 하나뿐이다** |
| `internal/validate/` | 검증 규칙 V1~V10 과 오류·경고 꼴 |
| `internal/assetindex/` | 주소 색인 읽기 · 주소/하위 찾기 · 낡음 판정 · 감옥 뿌리. **계약을 아는 유일한 곳** |
| `internal/assettest/` | asset 시험이 같이 쓰는 판 깔기 (시험에서만 부른다) |
| `internal/mpack/` · `internal/bake/` | 직접 쓴 MessagePack 인코더 / 16바이트 머리 + 본문 조립 |
| `internal/gen/` | C# 생성과 네임스페이스 치환 |
| `internal/migrate/` | 스키마 편집 ops 를 데이터 행에 옮긴다 (열 이름·enum 값). HTTP·디스크를 모르는 순수 함수 |
| `internal/serve/` | 로컬 서버와 API. 파일을 쓰는 곳은 표 PUT · 스키마 PUT · gen 셋이다. 에셋 파일은 `asset.go` 가 `os.Root` 로만 연다 |
| `ui/` | 표 편집 화면. `go:embed` 로 exe 안에 들어간다. 빌드 단계가 없다 (npm 없음) |
| `Unity/` | 손으로 쓴 C#. 생성물은 아니지만 `go:embed` 로 exe 에 들어가 `gen` 이 같이 내 준다 |
| `Unity/Verify/` | Unity 배치모드 검증 — `verify.ps1` · dll 목록(`Packages.psd1`) · 시험·빌드·탐침 C# · U4b 자료. `go:embed` 대상이 아니다 |
| `Testdata/` | 시험 자료. 무엇이 무엇인지는 `Testdata/README.md` |
| `Test/u6/` | 웹 UI 자동 판정 U6 — `run.ps1` · `gen.js`(2,000행 + 숫자 철자 행) · `cdp.js`(Node 내장 WebSocket CDP) · `u6.js`(시나리오). Go 시험이 아니다 — Node·크롬 없는 기계에서도 `go test` 가 안 깨진다 |
| `Docs/` | `Research` · `Design` · `Todo`. 설계는 `Docs/Design/2026-09-18-DataTool설계.md` |

## 2차에 비워 둔 자리

| 무엇 | 비워 둔 자리 |
| --- | --- |
| 암호화 | 머리 플래그 bit0 + 예약 6바이트. 명령은 `export --key-file <경로>` (이름만 잡아 뒀다) |
| 압축 | 같은 플래그 bit1. 차례는 압축 → 암호화 |
| 엑셀 가져오기 | `import --xlsx <파일> --table <이름>` — 가져오기는 늘 `fmt` 를 거쳐 쓴다 |
| 다국어 | `"loc": true` 가 붙은 열만 뽑는 `loc-export` |
| 표 쪼개 굽기 | `export --split` (Addressables 로 옮길 때) |

**1차에서는 이 자리에 대응하는 코드를 미리 안 짰다.** 안 쓰는 코드는 썩는다.

## 한계 (알고 두는 것)

- **MessagePack 인코더를 직접 썼다.** Go 시험(경계값 왕복)은 통과하지만 **C# 이 실제로 읽는지는
  Unity 에서 봐야 안다**(U2). 거기서 막히면 라이브러리로 간다.
- 성능을 안 쟀다. 「수천 행」은 어림이다. 표 하나 = 파일 하나라 쪼개기는 열려 있다.
- `gen` 은 **옛 생성 파일을 지우지 않는다.** 스키마에서 표를 없앴을 때 남는 `.cs` 는 목록으로 알리고 사람이 지운다.
- 생성 폴더 **밖에는 한 글자도 안 쓴다.** 그래서 생성 파일만 모이는 폴더를 따로 가른다.
- **CI · 새 PC 에는 `Library/` 가 없어 asset 검사(V10)가 경고 한 줄만 남기고 꺼진다.** 꼭 보려면 `--require-asset-index` 를 주고, 그 전에 `assettool index` 를 돌린다.
- **낡음 판정이 못 잡는 것 둘** — Addressables 폴더 항목 안에 파일만 새로 넣은 경우, 스프라이트 시트를 다시 자른 경우(`.meta` 만 바뀐다).
  둘 다 설정 `.asset` 파일이 안 바뀌어서다. 이상하면 `assettool index` 를 다시 돌린다.
- **옛 exe 는 `.datatool.json` 의 `assetIndex` 칸을 못 읽는다**(종료 4). 칸을 적었으면 새로 빌드한다.

## 알아 둘 것 (일부러 오류를 삼키는 자리 셋)

오류를 안 보는 자리는 이 셋뿐이고, 셋 다 **알릴 곳이 없어서** 그렇게 둔 것이다.

| 자리 | 무엇을 삼키나 | 왜 |
| --- | --- | --- |
| `internal/gen/write.go` 의 `defer os.Remove(tmpPath)` | 임시 파일 지우기 실패 | 이름 바꾸기가 끝났으면 그 파일은 이미 없다. 없어서 나는 실패라 알릴 것이 아니다 |
| `internal/serve/api.go` 의 `w.Write(out)` | 응답 본문 쓰기 실패 | 상태 코드를 이미 보낸 뒤다. 여기서 다른 상태 코드를 못 보낸다 |
| `internal/serve/asset.go` 의 `io.Copy(w, file)` | 에셋 파일 보내기 실패 | 같은 까닭 — 200 을 보낸 뒤다 |

남은 소단계와 차례는 `Docs/Todo/할일.md` 를 본다.

아래 「2. 경계 계약」 절은 연동 설계(스튜디오 `Docs/Design/2026-09-23-AssetTool과DataTool연동설계.md`) 2절을 **글자 그대로** 옮긴 것이다.
AssetTool README 에도 같은 글이 있다. 고칠 때는 두 곳을 같이 고친다.

## 2. 경계 계약 (계약 버전 1, 두 README 에 같은 글)

### 2-1. 색인 파일 `address-index.json`

AssetTool `index` 가 **쓰고**, DataTool·AssetTool 웹이 **읽는다.** UTF-8, BOM 없음. 쓸 때는 tmp → 이름 바꾸기라 반쯤 쓴 파일을 읽는 일이 없다.

**맨 위 칸**

| 칸 | 타입 | 필수 | 뜻 |
| --- | --- | --- | --- |
| `version` | int | ✓ | 계약 버전. 지금 `1` |
| `generator` | string | ✓ | 쓴 툴과 툴 버전 (`"assettool 0.1.0 (a1b2c3d)"`). 사람이 보는 값, 기계는 안 본다 |
| `generatedAt` | string | ✓ | 만든 시각, UTC, Go `time.RFC3339Nano` 꼴 (`2026-09-23T05:12:00Z`) |
| `unityRoot` | string | ✓ | 색인 파일이 있는 폴더 기준 Unity 뿌리 (`"../.."`). 다른 드라이브면 절대 경로 |
| `settingsPath` | string | ✓ | 읽은 `AddressableAssetSettings.asset` 자리 (뿌리 기준) |
| `sourceMtime` | string | ✓ | 읽은 설정 폴더(`settingsPath` 의 폴더) 안 `*.asset` 중 **가장 새 mtime**, UTC, `time.RFC3339Nano` |
| `labels` | string[] | ✓ | 라벨 전체 목록 (`m_LabelTable.m_LabelNames`). 없으면 `[]` |
| `entries` | object[] | ✓ | 항목. 차례는 아래 「쓰기 꼴」 |

**`entries[]` 한 칸**

| 칸 | 타입 | 필수 | 뜻 |
| --- | --- | --- | --- |
| `address` | string | ✓ | Addressables address 그대로 |
| `guid` | string | ✓ | 에셋 guid (32자 16진) |
| `path` | string | ✓ | 뿌리 기준 상대경로, `/` 구분, `Assets/` 또는 `Packages/` 로 시작. **빈 값 = 경로를 못 풀었다** |
| `kind` | string | ✓ | `image` · `audio` · `prefab` · `scene` · `other` 다섯 중 하나 (아래 표). 경로를 못 풀었으면 `other` |
| `group` | string | ✓ | 그룹 이름 (`m_GroupName`) |
| `includeInBuild` | bool | ✓ | 빌드에 들어가나 (4-4 규칙) |
| `labels` | string[] | ✓ | 이 항목의 라벨. 없으면 `[]` |
| `fromFolder` | string | — | 폴더 항목을 펼친 것이면 그 폴더의 address |
| `sub` | object[] | — | 하위 에셋 : `{"name": string, "rect": {"x","y","w","h": number}, "path"?: string, "guid"?: string}`. `rect` 는 픽셀, **y 는 아래에서 잰다**(Unity 꼴 그대로), **w 나 h 가 0 이면 그림 전체**(Single 스프라이트). **`sub` 가 없으면 「하위를 모른다」** 이지 「하위가 없다」가 아니다 (FBX·spriteatlas 하위도 `address[이름]` 으로 부른다) |

**`sub` 의 `path` · `guid` (계약 1판 덧붙임, 선택 칸)** — 스프라이트 아틀라스(`path` 가 `.spriteatlas` · `.spriteatlasv2` 인 `other` 항목)의 sub 는 스프라이트마다 원본 그림을 가리킬 수 있다.
`path` 가 있으면 `rect` 는 **그 파일 기준**, 없으면 항목 `path` 기준(지금의 스프라이트 시트)이다. `path` 규칙은 항목 `path` 와 같다(`Assets/`·`Packages/` 아래 정리된 `/` 상대경로).
DataTool 은 잘못된 sub `path` 를 항목 `path` 처럼 읽을 때는 받아 두고, 열 때 403 으로 막는다.

**`kind` 판정 (확장자, 소문자로 견준다)**

| kind | 확장자 |
| --- | --- |
| `image` | png jpg jpeg gif bmp tga psd psb tif tiff exr hdr |
| `audio` | wav mp3 ogg aif aiff flac xm mod it s3m |
| `prefab` | prefab |
| `scene` | unity |
| `other` | 나머지 전부 (`.asset` · `.mat` · `.spriteatlas` · `.fbx` …) |

**쓰기 꼴 (같은 입력이면 같은 바이트)**

- 차례 : `entries` 는 **address 바이트 차례**(Go `sort.Strings` 와 같다 — 대문자가 소문자 앞), address 가 같으면 guid 차례.
- 꼴 : Go `json.MarshalIndent(v, "", "  ")` 과 같은 들여쓰기(2칸) · HTML 이스케이프 안 함(`SetEscapeHTML(false)`) · 끝에 `\n` 하나 · 줄끝 LF. 칸 차례는 위 표 차례.
- 시각 : `time.RFC3339Nano` (UTC, 뒤쪽 0 은 Go 가 떼는 대로).

**그 밖의 규칙**

- 같은 address 가 두 항목에 있을 수 있다 (Addressables 가 막지 않는다). 색인은 둘 다 넣고, `index` 는 알림 한 줄을 낸다.
- **계약 버전 규칙 :** 같은 `version` 안에서는 **칸을 더하기만** 한다. 읽는 쪽은 모르는 칸을 무시한다. 칸을 빼거나 뜻을 바꾸면 `version` 을 올린다.
- **모르는 `version`** 이면 읽는 쪽은 추측하지 않고 멈춘다 — 「색인 계약 버전 N 을 모른다. DataTool 을 새로 빌드하거나 AssetTool 버전을 맞춰라」.

### 2-2. 예제 한 벌

```json
{
  "version": 1,
  "generator": "assettool 0.1.0 (a1b2c3d)",
  "generatedAt": "2026-09-23T05:12:00Z",
  "unityRoot": "../..",
  "settingsPath": "Assets/AddressableAssetsData/AddressableAssetSettings.asset",
  "sourceMtime": "2026-09-23T05:10:41.123456789Z",
  "labels": [
    "default",
    "ui"
  ],
  "entries": [
    {
      "address": "Hero",
      "guid": "0f1e2d3c4b5a69788796a5b4c3d2e1f0",
      "path": "Assets/Prefabs/Hero.prefab",
      "kind": "prefab",
      "group": "Default Local Group",
      "includeInBuild": true,
      "labels": [
        "default"
      ]
    },
    {
      "address": "Sfx/hit.wav",
      "guid": "1234567890abcdef1234567890abcdef",
      "path": "Assets/Audio/Sfx/hit.wav",
      "kind": "audio",
      "group": "Audio",
      "includeInBuild": true,
      "labels": [],
      "fromFolder": "Sfx"
    },
    {
      "address": "icons",
      "guid": "9a8b7c6d5e4f30211203f4e5d6c7b8a9",
      "path": "Assets/Art/icons.png",
      "kind": "image",
      "group": "UI",
      "includeInBuild": true,
      "labels": [
        "ui"
      ],
      "sub": [
        {
          "name": "icon_sword",
          "rect": {
            "x": 0,
            "y": 64,
            "w": 64,
            "h": 64
          }
        },
        {
          "name": "icon_potion",
          "rect": {
            "x": 64,
            "y": 64,
            "w": 64,
            "h": 64
          }
        }
      ]
    }
  ]
}
```

**이 파일은 `Testdata/contract/address-index.example.json` 으로 두 툴에 같은 바이트로 둔다.**
두 저장소 `.gitattributes` 에 `Testdata/contract/*.json -text` 를 더한다 — git 이 줄끝을 CRLF 로 바꾸면 바이트 대조가 깨진다.
DataTool 은 이것을 읽어 시험하고, AssetTool 은 시험 자료 프로젝트를 색인해 **이것과 바이트가 같은지** 본다(`generatedAt`·`generator`·`sourceMtime` 은 시험이 고정값으로 넣는다).
계약을 고치면 두 파일을 같은 날 고치고, 양쪽 README 의 이 절도 같이 고친다.

### 2-3. 썸네일 폴더 (2차부터 쓴다, 자리는 지금 정한다)

| 규칙 | 값 |
| --- | --- |
| 자리 | `<Unity 뿌리>/Library/AssetTool/thumbs/<guid>.png` — **색인 자리와 상관없이 늘 여기** |
| 파일 이름 | 에셋 guid 소문자 32자 + `.png`. address 가 아니라 guid 인 까닭 : address 는 바뀌어도 guid 는 안 바뀐다 |
| 크기 · 꼴 | **128×128 정사각, RGBA PNG, 투명 배경.** 물체는 경계 상자를 가운데 맞춰 채운다 |
| 누가 | AssetTool Unity 쪽이 쓴다 / DataTool·AssetTool 웹은 **있으면 보이고 없으면 「썸네일 없음」** |

### 2-4. 칸 값 규칙

`asset` 칸 값 = address 그대로. 하위 에셋은 `address[이름]` (Addressables 하위 객체 문법). **빈 문자열은 「없음」** 이다. 라벨은 안 받는다.

