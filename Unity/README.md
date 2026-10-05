# Unity 쪽 붙이기

이 폴더의 파일 셋은 **datatool 이 만든 것이 아니라 손으로 쓴 원본**이다.
**손으로 복사하지 않는다 — `datatool gen` 이 같이 내 준다.**

| 파일 | 하는 일 | 생성 폴더에 나오는 이름 |
| --- | --- | --- |
| `GameDataLoader.cs` | `gamedata.bytes` 의 머리 16바이트를 검사하고 본문을 `GameDataTables` 로 넘긴다 | `GameDataLoader.cs` |
| `GameDataException.cs` | 데이터가 어긋났을 때 던지는 예외 | `GameDataException.cs` |
| `Officina.Data.asmdef` | 이 코드와 생성 코드가 같이 사는 어셈블리 | `<namespace>.asmdef` |

`gen` 은 이 셋을 exe 안(`go:embed`)에서 꺼내 **`Officina.Data` 를 `schema.json` 의 `namespace` 값으로
갈아 끼우고**(asmdef 의 `name`·`rootNamespace` 도) 생성 폴더에 같이 쓴다. 치환 뒤 `Officina.` 이
한 글자라도 남으면 파일·줄을 알리고 멈춘다 — **잔여 0 을 사람이 아니라 코드가 센다.**

그래서 생성 폴더의 이 셋도 **사람이 고치지 않는다.** 고칠 것이 있으면 `DataTool/Unity/` 의
원본을 고치고 `datatool gen` 을 다시 돌린다. asmdef 에는 JSON 이라 주석을 못 달아
`"_generated"` 칸으로 같은 경고를 적어 둔다 (Unity 는 모르는 칸을 조용히 넘긴다).

## 넣는 차례

1. **MessagePack-CSharp 3.1.9 를 넣는다.** 기본 길은 **nupkg 의 dll 을 `Assets/Plugins/MessagePack/` 에 직접 넣기**다
   (Unity 2022.3.12f1 이상 — IL2CPP 를 C# 소스 제너레이터로 받치는 것이 그 판부터다).

   | 파일 | 패키지 · 판 |
   | --- | --- |
   | `MessagePack.dll` | MessagePack 3.1.9 (`lib/netstandard2.1`) |
   | `MessagePack.Annotations.dll` | MessagePack.Annotations 3.1.9 |
   | `Microsoft.NET.StringTools.dll` | Microsoft.NET.StringTools 17.11.4 |
   | `System.Collections.Immutable.dll` | System.Collections.Immutable 8.0.0 |
   | `System.Runtime.CompilerServices.Unsafe.dll` | System.Runtime.CompilerServices.Unsafe 6.0.0 ← **빼면 컴파일은 되는데 float 열을 읽을 때 터진다** |
   | `Analyzers/MessagePack.SourceGenerator.dll` + `.meta` | MessagePackAnalyzer 3.1.9 (`analyzers/roslyn4.3/cs`) — `.meta` 는 `Verify/Meta/` 의 것을 쓴다 (`RoslynAnalyzer` 라벨) |

   - **손으로 안 받아도 된다** — `Verify/verify.ps1 -Project <프로젝트> -PackagesOnly` 가
     nuget.org 에서 받아 SHA256 을 대조하고(`Verify/Packages.psd1`) 위 자리에**만** 넣는다.
     에디터가 열려 있어도 되고, 생성 코드·구운 파일·시험 코드는 건드리지 않는다.
     다른 판 dll · 목록 밖 dll · 내용이 다른 분석기 `.meta`(줄 끝만 다른 것은 같다고 본다)가 이미 있으면
     프로젝트에는 아무것도 안 쓰고 멈춘다(종료 2). nupkg 캐시와 `-LogDir` 의 `pkg` 에는 쓴다.
     파일은 `.meta` 먼저, 같은 폴더의 숨은 tmp(`.이름.part~`)에 쓴 뒤 바꿔 넣는다 — 열린 에디터가 반쯤 쓰인 dll 을 안 가져간다.
     `-LogDir` 가 프로젝트 아래면 종료 2 로 막는다 (dll 사본이 `Assets` 에 남으면 같은 어셈블리가 둘이 된다).
   - **스위치 없이 돌리는 verify 는 시험 프로젝트용이다** — 게임 프로젝트에 바로 돌리지 않는다.
   - 다른 길 : NuGetForUnity 로 `MessagePack` 설치. 판은 3.1.9 로 맞춘다.
     NuGetForUnity 로 `Assets/Packages` 에 이미 넣었으면 `-PackagesOnly` 는 그 중복 dll 을 못 막는다 (`Plugins/MessagePack` 만 본다).
   - git URL 패키지(`MessagePack.Unity`)는 **Unity 타입(Vector3·Color 등)을 직렬화할 때만** 넣는다. 우리 행 클래스는 안 쓴다.
     넣을 때는 `#v3.1.9` 로 판을 고정한다.
   - 근거 : <https://github.com/MessagePack-CSharp/MessagePack-CSharp#unity-support> · `Docs/Research/2026-10-03-Unity헤드리스검증조사.md`
2. **생성 폴더를 만든다** — `Assets/_Project/Scripts/Data/Generated/`.
   `datatool gen` 이 이 폴더 안만 쓴다. 로더 셋도 여기 같이 나온다.
   **사람이 여기 있는 파일을 고치지 않는다** (다음 gen 에서 덮인다).
3. `datatool gen` 을 돌린다. 복사할 것이 없다.
4. **게임 코드가 asmdef 안에 있으면** 그 asmdef(런타임·에디터 둘 다)의 `references` 에 생성 asmdef 이름
   (= 스키마 `namespace`, 예 `Officina.Data`)을 더한다. `autoReferenced` 는 asmdef 없는 코드(Assembly-CSharp)에만 듣는다.
5. **구운 파일 자리** — `Assets/StreamingAssets/gamedata.bytes` (`datatool export` 산출물).
   Resources 로 읽고 싶으면 아래 「첫 호출」의 Resources 줄을 본다.
6. **리졸버를 손으로 엮지 않는다.** `StandardResolver` 가 생성 어셈블리의 `GeneratedMessagePackResolver` 를 찾아 쓴다
   (Win64 IL2CPP · 6000.3.23f1 · 스트리핑 Minimal/High 에서 확인, 2026-10-03). 모바일에서 포매터를 못 찾으면 그때
   `[GeneratedMessagePackResolver] partial class …` 와 `StaticCompositeResolver` 를 얹는다.

## 첫 호출

```csharp
using Officina.Data;   // ← 치환한 네임스페이스

GameDataTables data = GameDataLoader.LoadFromStreamingAssets();

ItemRow sword = data.Item["sword_iron"];
Debug.Log(sword.Name + " / " + sword.Grade);      // enum 은 Grade 속성으로 읽는다

DropRow drop = data.DropRows[0];
ItemRow dropped = data.Get<ItemRow>(drop.ItemId); // ref 열은 Get<T> 로 잇는다
```

- **안드로이드**에서는 `StreamingAssets` 가 압축된 jar 안이라 `File` 로 못 읽는다.
  `UnityWebRequest` 로 바이트를 받아 `GameDataLoader.Load(byte[])` 를 부른다.
- **Resources 로 읽어도 된다** (동기 · 안드로이드·WebGL 도 같은 코드). `.datatool.json` 의 `export` 를
  `../Assets/<프로젝트>/Resources/Data/gamedata.bytes` 로 두고
  `GameDataLoader.Load(Resources.Load<TextAsset>("Data/gamedata").bytes)` 를 부른다.
  `export` 는 데이터 폴더 기준 상대경로다.
- `enum` 열은 구운 파일에 **이름 문자열**로 들어간다. 그래서 행에는 `GradeName`(문자열)과
  `Grade`(enum) 둘이 있다. 읽을 때는 `Grade` 를 쓴다. 리플렉션을 쓰는 enum 리졸버를 피하려고
  이렇게 했다 — IL2CPP 에서 그게 제일 잘 터진다.
- 스키마만 고치고 안 구웠으면 `Load` 에서 **바로 예외**가 난다 (`schemaHash` 가 다르다).

## 확인 차례

앞이 막히면 뒤는 의미가 없다. 이 차례로 본다. **`Verify/verify.ps1` 이 사람 대신 다 판정한다** (아래 「배치모드 검증」).

| # | 무엇 | 합격 |
| --- | --- | --- |
| U1 | 생성 `.cs` 를 넣고 Unity 를 연다 | 컴파일 오류 0 · 소스 제너레이터가 포매터를 만든다 |
| U2 | `gamedata.bytes` 를 넣고 `Load` 를 부른다 | **Go 가 구운 것을 C# 이 읽는다** ← 진짜 관문 |
| U3 | 값 몇 개 눈으로 대조 | 한글이 안 깨진다 · float 이 안 어긋난다 · enum 이 맞다 |
| U4 | 스키마만 고치고 안 구운 채 실행 | `GameDataException` 이 난다 |
| U4b | 열 타입을 바꾼 스키마로 구운 옛 파일을 읽는다 | MessagePack 예외가 아니라 `GameDataException` 이 난다 |
| U5 | IL2CPP 빌드 | 리플렉션 없이 돈다 |

## 배치모드 검증 (`Verify/`)

**빈 시험 프로젝트나 사본에서만 돌린다 — Generated · StreamingAssets · Plugins/MessagePack 을 덮어쓴다.**
그 자리에 우리 것이 아닌 `.cs` 나 다른 판 dll 이 보이면 아무것도 쓰기 전에 멈춘다(종료 2).
게임 프로젝트에는 `-PackagesOnly` 만 쓴다 (dll 만 넣는다 · 위 「넣는 차례」 1번).

```powershell
.\Verify\verify.ps1 -Project <Unity 프로젝트> [-DataTool <exe>] [-SkipTests] [-SkipBuild] [-SkipPlayer] [-Stripping High] [-EditorTimeoutMin 30] [-Clean]
.\Verify\verify.ps1 -Project <게임 프로젝트> -PackagesOnly [-LogDir <프로젝트 밖 폴더>]   # 다른 인자와 같이 주면 종료 2
```

- 에디터는 `ProjectSettings/ProjectVersion.txt` 의 판으로 `C:\Program Files\Unity\Hub\Editor\<판>\Editor\Unity.exe` 를 찾는다 (`-UnityExe` 가 이긴다).
  **에디터를 닫고** 돌린다.
- 차례 : 툴 굽기 → dll 넣기 → `Testdata/table/ok` gen·export → 시험·빌드 코드 복사 → EditMode(U1~U4b) → Win64 IL2CPP 빌드 → 플레이어(U5). 끝에 판정 표.
- 종료 코드 : **0 통과 · 1 판정 실패(코드를 고칠 일) · 2 환경 문제(기계를 고칠 일).**
  - 1 : U1~U5 중 하나가 실패했거나, 에디터·빌드가 0 이 아닌 코드로 끝났다.
  - 2 : 에디터 없음·열려 있음 · nupkg 해시 다름 · 남의 파일 발견 · 잡지 않은 예외(네트워크·zip 등) ·
    에디터 한 번이 `-EditorTimeoutMin`(기본 30분)을 넘김 · 시험 결과 XML 도 `error CS` 도 없음(라이선스 등).
- `-DataTool`·`-SkipToolBuild` 로 받은 exe 의 커밋이 지금 소스 커밋과 다르면 경고 한 줄만 낸다(멈추지 않는다).
- EditMode 는 `-assemblyNames DataToolVerify.Tests` 로 Verify 시험만 돌린다.
- 넣는 자리 : `Assets/Plugins/MessagePack/` · `Assets/_Project/Scripts/Data/Generated/` · `Assets/StreamingAssets/gamedata.bytes` ·
  이름에 `DataToolVerify` 가 붙은 셋(`Assets/Tests/` · `Assets/Editor/` · `Assets/DataToolVerify/`). `-Clean` 은 그 셋만 지운다.
- 빌드 동안 Standalone `scriptingBackend` 를 IL2CPP 로(`-Stripping` 을 주면 스트리핑도) 바꾸고, **끝나면 바꾸기 전 값으로 되돌려 `ProjectSettings` 를 저장한다.**
  구운 플레이어는 그대로 남는다. Unity 가 `m_BuildTargetBatching` 에 Standalone 줄을 더하는 것은 되돌리지 않는다.
  시간 넘김으로 에디터를 죽이면 되돌리지 못할 수 있다.
- 플레이어 탐침은 `-datatoolVerify` 인자가 있을 때만 돈다. 게임 빌드에 섞여도 아무 일 안 한다.
- 로그·플레이어는 `%TEMP%\datatool-verify-<시각>` 에 남는다. **플레이어 폴더가 약 1.3GB** 라 다 본 뒤 지운다.
- 2026-10-03 실측 (빈 2D URP 프로젝트 · 6000.3.23f1) : 판정 다섯 다 통과 · 종료 0. EditMode 17~55초 · 첫 빌드 260초(캐시 뒤 26초) · 플레이어 3초.
