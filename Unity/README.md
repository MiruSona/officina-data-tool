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

1. **MessagePack-CSharp 를 설치한다.** 둘 다 해야 한다 (Unity 2022.3.12f1 이상 — IL2CPP 를
   C# 소스 제너레이터로 받치는 것이 그 판부터다).
   - NuGetForUnity 로 `MessagePack` 패키지 설치 (`Window → NuGet → Manage NuGet Packages`)
   - Package Manager 에서 `Add package from git URL` :
     `https://github.com/MessagePack-CSharp/MessagePack-CSharp.git?path=src/MessagePack.UnityClient/Assets/Scripts/MessagePack`
   - 근거 : <https://github.com/MessagePack-CSharp/MessagePack-CSharp#unity-support>
2. **생성 폴더를 만든다** — `Assets/_Project/Scripts/Data/Generated/`.
   `datatool gen` 이 이 폴더 안만 쓴다. 로더 셋도 여기 같이 나온다.
   **사람이 여기 있는 파일을 고치지 않는다** (다음 gen 에서 덮인다).
3. `datatool gen` 을 돌린다. 복사할 것이 없다.
4. **구운 파일 자리** — `Assets/StreamingAssets/gamedata.bytes` (`datatool export` 산출물).
5. MessagePack.Unity 가 뜰 때 `MessagePackSerializer.DefaultOptions` 를 스스로 잡아 준다.
   따로 리졸버를 엮을 일은 없다. IL2CPP 빌드에서 포매터를 못 찾으면 그때
   `[GeneratedMessagePackResolver] partial class …` 와 `StaticCompositeResolver` 를 얹는다 (U5).

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
- `enum` 열은 구운 파일에 **이름 문자열**로 들어간다. 그래서 행에는 `GradeName`(문자열)과
  `Grade`(enum) 둘이 있다. 읽을 때는 `Grade` 를 쓴다. 리플렉션을 쓰는 enum 리졸버를 피하려고
  이렇게 했다 — IL2CPP 에서 그게 제일 잘 터진다.
- 스키마만 고치고 안 구웠으면 `Load` 에서 **바로 예외**가 난다 (`schemaHash` 가 다르다).

## 확인 차례 (사람이 봐야 아는 것)

앞이 막히면 뒤는 의미가 없다. 이 차례로 본다.

| # | 무엇 | 합격 |
| --- | --- | --- |
| U1 | 생성 `.cs` 를 넣고 Unity 를 연다 | 컴파일 오류 0 · 소스 제너레이터가 포매터를 만든다 |
| U2 | `gamedata.bytes` 를 넣고 `Load` 를 부른다 | **Go 가 구운 것을 C# 이 읽는다** ← 진짜 관문 |
| U3 | 값 몇 개 눈으로 대조 | 한글이 안 깨진다 · float 이 안 어긋난다 · enum 이 맞다 |
| U4 | 스키마만 고치고 안 구운 채 실행 | `GameDataException` 이 난다 |
| U5 | IL2CPP 빌드 | 리플렉션 없이 돈다 |
