# ProjectMozzi 적용 피드백 (2026-10-03)

게임 저장소(ProjectMozzi2)의 표 7장 · 58행을 `JsonUtility` 방식에서 DataTool 로 갈아타며 겪은 것이다.
datatool 0.1.0-dev (커밋 70665c0) · Unity 6000.3.23f1 · MessagePack 3.1.9.

## 잘 된 것

- `validate` → `fmt` → `gen` → `export` 가 **첫 판에 다 통과**했고, 에디터에서 `GameDataLoader.Load(byte[])` 가 바로 읽었다 (U2). 옛 JSON 과 451칸을 견줘 다른 칸 0.
- `export --out` 대신 `.datatool.json` 의 `export` 칸에 `../Assets/_Project/Resources/Data/gamedata.bytes` 를 적으니 **Resources 자리로 바로 구워졌다.** `Resources.Load<TextAsset>` + `Load(byte[])` 로 동기 로드가 된다.
- `gen` 이 로더 · asmdef 까지 네임스페이스를 갈아 내 줘서 복사할 것이 없었다.
- 필수 `string` 열에 `""` 가 통과해서 「없으면 빈 문자열」 열을 그대로 옮길 수 있었다.

## 막힌 것 · 헷갈린 것

1. **`verify.ps1` 로 dll 만 받는 길이 실제 게임 프로젝트에서는 안 된다.**
   `Unity/README.md` 는 「손으로 안 받아도 된다 — `verify.ps1 -SkipTests -SkipBuild -SkipPlayer`」 라고 하지만,
   그 명령은 ① 에디터가 열려 있으면 종료 2 ② `Testdata/table/ok` 의 생성 코드와 `StreamingAssets/gamedata.bytes` 를 게임 프로젝트에 써 넣는다.
   이번에는 **빈 폴더에 `ProjectVersion.txt` 만 둔 가짜 프로젝트**에 `-SkipToolBuild -SkipData -SkipCopy -SkipTests -SkipBuild -SkipPlayer` 로 돌려 dll 여섯 장을 받고 손으로 옮겼다.
   → 바람 : dll 만 받아 넣는 스위치(`-PackagesOnly` 같은 것, 에디터가 열려 있어도 됨) 또는 `datatool` 명령 하나.
2. **Resources 로 읽는 길이 README 에 없다.** `Unity/README.md` 는 StreamingAssets 와 「안드로이드는 UnityWebRequest」 만 적는다.
   WebGL 도 `File` 로 못 읽는다. `.datatool.json` 의 `export` 를 `Resources/…/gamedata.bytes` 로 두고 `TextAsset.bytes` 를 넘기는 길을 한 줄 적어 주면 좋겠다 (동기라 코드가 제일 짧다).
3. **표·열 이름이 소문자 밑줄만 된다는 것을 README 에서 못 찾았다.** `id` 값의 정규식만 적혀 있다.
   `MozziSpecies` · `fenceMax` 를 쓰던 프로젝트는 이름을 다 바꿔야 하므로 「데이터 꼴」 절에 한 줄 있으면 좋겠다 (소스 `internal/schema/parse.go` 를 읽고 알았다).
4. **`ref` 열은 빈 문자열을 못 받는다.** 「없을 수도 있는 참조」(`next_species_id` · `reveal_ingredient_id`)는 `string` 으로 낮춰야 해서 참조 검사를 잃는다.
   → 바람 : `ref` 에 `default: ""` 를 주면 빈 값은 건너뛰고 값이 있을 때만 V6 을 보는 꼴.
5. **생성 asmdef 는 다른 asmdef 가 직접 참조해야 한다.** `autoReferenced: true` 는 asmdef 없는 코드에만 듣는다.
   게임 런타임 · 에디터 asmdef 둘 다 `references` 에 네임스페이스 이름을 더해야 했다. 「넣는 차례」에 한 줄 있으면 덜 헤맨다.
6. `list<T>` 가 `T[]` 로 나와 `List<string>` 을 쓰던 호출부의 `.Count` 를 `.Length` 로 바꿔야 했다. 문제는 아니고 옮길 때 알아 둘 것이다.
