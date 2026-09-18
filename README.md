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
| `fmt` | 데이터 JSON 을 규칙대로 다시 쓴다 (열 차례 · 한 줄 한 행 · 기본값 빼기). **스키마에 없는 열이 있으면 한 글자도 안 쓰고 종료 2** — 다시 쓰면 그 값이 사라지기 때문이다. `--check` 는 한 글자도 안 쓰고 바뀔 파일만 알린다(있으면 종료 2) | **쓴다** — 데이터 JSON |
| `validate` | 스키마와 데이터를 검사한다 (V1~V9). 첫 건에서 안 멈추고 다 모아서 낸다 | 안 쓴다 |
| `export` | `gamedata.bytes` 를 굽는다. **먼저 validate** | **쓴다** — `--out` 자리 하나 |
| `gen` | C# 을 만든다 (`<표>Row.cs` · `<enum>.cs` · `GameDataTables.cs`) **+ `Unity/` 의 로더 셋을 네임스페이스 치환해 같이 낸다**. **먼저 validate** | **쓴다** — 생성 폴더 안만 |
| `serve` | 표 편집 UI 를 `127.0.0.1` 에만 띄운다. 저장은 행 배열을 엔진에 넘겨 검증 뒤 파일을 다시 쓴다 | **쓴다** — UI 가 저장할 때만 |
| `version` | 판과 빌드 시각 | 안 쓴다 |

전역 옵션은 둘뿐이다.

- `--data DIR` — 데이터 폴더(`GameData`).
- `--json` — 결과를 JSON 한 덩어리로 낸다 (AI·스크립트가 부르기 좋게). 모든 명령이 받는다.

### 데이터 폴더를 어떻게 찾나

`--data` 를 안 주면 **지금 폴더에서 위로 올라가며 `.datatool.json` 을 찾는다.**
한 칸마다 그 폴더 자신과 그 아래 `GameData/` 둘을 본다 — 설정은 `GameData/` 안에 살기 때문이다.
못 찾으면 폴더를 지어내지 않고 **종료 1 과 안내**다.

`.datatool.json` 은 칸이 둘뿐이고 둘 다 **데이터 폴더 기준 상대경로**다.

```json
{
  "gen": "../Assets/_Project/Scripts/Data/Generated",
  "export": "../Assets/StreamingAssets/gamedata.bytes"
}
```

- 이 값이 `export` · `gen` 의 기본 `--out` 이다. **명령에 `--out` 을 주면 그것이 이긴다.**
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
| `GET /api/schema` | 스키마 전체 (UI 가 이것으로 열과 편집기를 만든다) |
| `GET /api/tables` | 표 이름과 행 수 |
| `GET /api/table/{이름}` | 행 배열. 기본값이라 파일에서 빠진 열은 서버가 채워서 준다 |
| `PUT /api/table/{이름}` | 행 배열 통째 → 검증 → 통과하면 tmp→이름바꾸기로 저장 (걸리면 400) |
| `POST /api/validate` | `validate` 와 같은 일. 한 글자도 안 쓴다 |

## 종료 코드

| 코드 | 뜻 |
| --- | --- |
| 0 / 1 | 성공 / 사용법 잘못 (모르는 인자 · 데이터 폴더를 못 찾음 · `init` 이 덮으려 함) |
| **2** | **데이터 검증 실패** (타입 · 필수 · 참조 · 중복 · 범위 · `fmt --check` 가 바뀔 파일을 찾음) |
| **3** | **스키마 자체가 틀림** (모르는 타입, 없는 enum, 첫 열이 `id` 가 아님) |
| 4 / 5 | 읽기 실패 (없음 · JSON 깨짐 · 설정에 모르는 칸) / 쓰기 실패 |

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

## 스키마 타입

`GameData/schema.json` 한 파일이 표 전부를 담는다. **첫 열은 반드시 `id`, 타입 `string`** 이다.
열 칸은 `name` · `type` · `default` · `min` · `max` · `enum` · `ref` · `loc` · `desc` 아홉이 전부다.

| 타입 | JSON | C# | 비고 |
| --- | --- | --- | --- |
| `int` · `float` · `bool` | 숫자 · 참거짓 | `int` · `float` · `bool` | `float` 은 굽을 때 float32 |
| `string` | 문자열 | `string` | `"loc": true` 를 달 수 있다 (1차는 무시 — 2차 다국어 표시) |
| `enum` | 목록 안의 문자열 | 생성된 `enum` | `enum` 칸 필수 |
| `ref` | 다른 표의 `id` | `string` | 로더가 딕셔너리로 이어 준다 |
| `list<T>` | 배열 | `T[]` | `T` 는 위 여섯 중 하나. **중첩 `list` 는 안 된다** |

`default` 가 없고 값도 없으면 **필수 열**이다. `required` 칸을 따로 안 만든다.
날짜·시간·벡터·중첩 문서·수식은 없다 — 필요하면 **표를 하나 더 만든다**로 푼다.

## Unity 에 붙이기

`Unity/` 의 손으로 쓴 `.cs` 둘과 `.asmdef` 는 **`gen` 이 exe 안에서 꺼내 네임스페이스를 갈아 끼워
생성 폴더에 같이 써 낸다.** 사람이 복사하고 사람이 치환하면 「`Officina.` 잔여 0」을 아무도 안 세기 때문이다.
MessagePack-CSharp 설치, 첫 호출, 확인 차례(U1~U5)는 **`Unity/README.md`** 를 본다.

## 폴더

| 자리 | 무엇 |
| --- | --- |
| `cmd/datatool/` | 인자 가르기와 결과 찍기만 한다 |
| `internal/schema/` | `schema.json` 읽기·검사·`schemaHash`. **타입 목록이 여기 하나뿐이다** |
| `internal/table/` | 데이터 JSON 읽기·쓰기. **「한 줄 한 행」을 아는 유일한 곳** |
| `internal/textfile/` | 읽기 입구 셋(표·스키마·설정)이 같이 쓰는 파일 읽기. **BOM 걷기가 여기 하나뿐이다** |
| `internal/validate/` | 검증 규칙 V1~V9 와 오류 꼴 |
| `internal/mpack/` · `internal/bake/` | 직접 쓴 MessagePack 인코더 / 16바이트 머리 + 본문 조립 |
| `internal/gen/` | C# 생성과 네임스페이스 치환 |
| `internal/serve/` | 로컬 서버와 API. 파일을 쓰는 곳은 `PUT` 하나뿐이다 |
| `ui/` | 표 편집 화면. `go:embed` 로 exe 안에 들어간다. 빌드 단계가 없다 (npm 없음) |
| `Unity/` | 손으로 쓴 C#. 생성물은 아니지만 `go:embed` 로 exe 에 들어가 `gen` 이 같이 내 준다 |
| `Testdata/` | 시험 자료. 무엇이 무엇인지는 `Testdata/README.md` |
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

## 알아 둘 것 (일부러 오류를 삼키는 자리 둘)

오류를 안 보는 자리는 이 둘뿐이고, 둘 다 **알릴 곳이 없어서** 그렇게 둔 것이다.

| 자리 | 무엇을 삼키나 | 왜 |
| --- | --- | --- |
| `internal/gen/write.go` 의 `defer os.Remove(tmpPath)` | 임시 파일 지우기 실패 | 이름 바꾸기가 끝났으면 그 파일은 이미 없다. 없어서 나는 실패라 알릴 것이 아니다 |
| `internal/serve/api.go` 의 `w.Write(out)` | 응답 본문 쓰기 실패 | 상태 코드를 이미 보낸 뒤다. 여기서 다른 상태 코드를 못 보낸다 |

남은 소단계와 차례는 `Docs/Todo/할일.md` 를 본다.
