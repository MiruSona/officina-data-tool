# 시험 자료

Go 시험이 읽는 자료다. **판 하나는 작게 둔다** — 시험 자료가 커지면 아무도 안 읽는다.
바른 판 하나에 **일부러 깨뜨린 판**을 규칙마다 하나씩 붙이는 꼴이다.

| 폴더 | 무엇을 위한 자료인가 | 누가 읽나 |
| --- | --- | --- |
| `schema/ok/` | 바른 스키마 한 장 (표 셋 · enum 둘 · 타입 여덟(asset 포함)을 한 번씩 쓴다) | `internal/schema` · `internal/gen` |
| `schema/broken/` | **스키마가 틀린 판 스물넷** — 모르는 타입 · 없는 enum · 첫 열이 `id` 아님 · 중첩 `list` · asset 이 아닌 열의 `kind` · 모르는 `kind` 등. 파일 이름이 곧 무엇이 틀렸는지다. 다 종료 3 이다 (`broken-json` 만 읽기 실패 4) | `internal/schema` (T1) |
| `table/ok/` | 규칙대로 적힌 데이터 셋 — `item` 6행 · `monster` 4행 · `drop` 8행. **읽고 그대로 쓰면 바이트가 같아야 한다** | `internal/table`(T2) · `internal/bake`(T6) · `cmd/datatool` |
| `table/messy/` | 같은 내용인데 **일부러 흐트러뜨린 판** — 열 차례가 섞이고, 한 줄 한 행이 아니고, 기본값이 적혀 있다. `fmt` 가 이것을 `table/ok/` 꼴로 되돌린다 | `internal/table`(T2) · `fmt` 시험 |
| `validate/` | 검증 시험의 바탕. `schema.json` 과 바른 데이터 셋(`ok/`)을 임시 폴더에 깐 뒤 그 위에 깨진 파일만 덮는다 | `internal/validate` · `cmd/datatool` |
| `validate/v2-…` ~ `v9-…` | **규칙마다 깨뜨린 판 하나씩.** 폴더 이름이 규칙 번호다 — `v2` 필수·모르는 열 · `v3` 타입 · `v4` 범위 · `v5` enum · `v6` ref · `v7` id 중복·꼴 · `v9` list | `internal/validate` (T3·T4) |
| `validate/many/` | 오류가 여럿 든 판. **첫 건에서 안 멈추고 다 모아서 내는지**를 본다 | `internal/validate` |
| `contract/address-index.example.json` | AssetTool 과의 **경계 계약 예제** (연동 설계 2-2 와 바이트까지 같다). `-text` 라 줄바꿈이 안 바뀐다. AssetTool 쪽 예제와 sha256 이 같아야 한다 | `internal/assetindex` |
| `asset/` | asset 열 시험 판. `unity/` 는 Unity 뿌리 흉내(작은 png 한 장 · wav 한 장 · psd 흉내 · Addressables 설정 하나), `address-index.json` 은 시험용 색인(탈출 경로 다섯 포함), `schema.json` · `card.json` 은 데이터 폴더. `internal/assettest` 가 임시 폴더에 깐다 | `internal/validate` · `internal/serve` · `cmd/datatool` |
| `roundtrip-cs/` | C# 이 Go 가 구운 `gamedata.bytes` 를 읽는지 보는 콘솔 판. 쓰는 법은 그 안 `README.md` | 사람 (`dotnet run`) |
| `gen/expected/` | C# 생성의 **정답지 여섯 장.** `gen` 이 낸 것과 글자까지 같아야 한다 (두 번 돌려도 같다 — 생성 시각을 안 박는다) | `internal/gen` (T7) |

## 손댈 때

- **`table/ok/` 를 고치면 `table/messy/` 도 같이 고친다.** 둘은 「같은 내용, 다른 꼴」이라는 짝이다.
- **스키마를 고치면 `gen/expected/` 여섯 장이 같이 바뀐다.** 눈으로 diff 를 보고 고친다 —
  정답지를 생성물로 덮어 버리면 시험이 아무것도 안 지키게 된다.
- **`contract/` 예제는 손대지 않는다.** 고치면 계약이 바뀐 것이라 연동 설계 · AssetTool 쪽을 같이 고쳐야 한다.
- 시험은 **임시 폴더에 복사해 놓고 돌린다.** 여기 있는 파일은 시험이 끝나도 안 바뀐다.
