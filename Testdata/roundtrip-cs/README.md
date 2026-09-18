# C# 왕복 검증 (Unity 없이)

**Go 가 구운 `gamedata.bytes` 를 진짜 MessagePack-CSharp 가 읽는가** — 이 툴에서 제일 큰
미확인 위험(설계 13장 · `Unity/README.md` 의 U2·U3·U4)을 Unity 를 안 켜고 미리 본다.

Unity 프로젝트 대신 net9.0 콘솔 한 판을 만들고, **`datatool gen` 이 낸 골든 한 벌을 복사하지 않고
그대로 컴파일**한다. 그러니 여기서 통과하면 Unity 에서 볼 것은 IL2CPP(U5) 뿐이다.

| 파일 | 하는 일 |
| --- | --- |
| `Roundtrip.csproj` | `MessagePack` 패키지를 받고 `../gen/expected/*.cs` 를 링크해서 컴파일한다 |
| `Program.cs` | 읽어서 값 52 가지를 기대값과 대조한다. 하나라도 어긋나면 종료 1 |

`gen` 이 행 클래스·enum·`GameDataTables` 뿐 아니라 **로더(`GameDataLoader`·`GameDataException`)까지
네임스페이스를 맞춰 같이 내므로**, 여기서 파일을 베끼거나 네임스페이스를 손볼 일이 없다.
`MyGame.Data.asmdef` 는 Unity 것이라 컴파일에 안 들어간다.

## 무엇을 대조하나

행 수 · id · 한글 이름 · int · float(0.05 같은 값이 float32 로 그대로) · bool ·
enum 이름 문자열 → enum 값 · `list<string>` · `list<int>` · **빠진 열이 기본값으로 채워졌나**
(atk 없는 행이 0, tags 없는 행이 빈 배열, counts 없는 행이 `[1]`) · ref 열로 `Get<T>` 가 이어지나 ·
`schemaHash` 가 다른 파일이 `GameDataException` 을 내나.

## 돌리는 법

```powershell
cd DataTool\Testdata\roundtrip-cs
..\..\bin\datatool.exe export --data ..\table\ok --out $env:TEMP\gamedata.bytes
dotnet run -- $env:TEMP\gamedata.bytes 81f25b44dda721be [해시가 다른 파일]
```

셋째 인자(해시를 일부러 망친 파일)는 없으면 그 항목만 건너뛴다. 만들 때는 구운 파일 안의
`81f25b44dda721be` 열여섯 글자를 `0000000000000000` 으로 덮으면 된다 (길이가 같아 자리가 안 밀린다).

## 마지막 결과 (2026-09-18)

- .NET 9.0.10 (SDK 10.0.400) · MessagePack **3.1.9** · 빌드 **경고 0 · 오류 0**
- `dotnet run` → **확인 52건 · 어긋남 0건 · 종료 0**. 한글·float32·enum·기본값·ref·예외 모두 맞았다.
- MessagePack 3 의 소스 제너레이터(`MessagePackAnalyzer` 가 딸려 온다)가 골든 코드에서
  `ItemRowFormatter`·`MonsterRowFormatter`·`DropRowFormatter` 와 `GeneratedMessagePackResolver`
  를 **경고 없이** 만들어 냈다 — 리플렉션 없는 IL2CPP(U5) 쪽 신호가 좋다.
  다만 그 리졸버를 `StaticCompositeResolver` 에 끼우는 것은 여전히 사람 몫이다 (`Unity/README.md` 5번).
- 처음에는 손으로 쓴 `Unity/*.cs` 를 링크하느라 네임스페이스를 베끼며 치환하는 단계가 있었다.
  `gen` 이 로더까지 내주게 되면서 **그 함정 자체가 사라졌다.**
