# verify.ps1 이 Unity 프로젝트의 Assets/Plugins/MessagePack/ 에 넣는 dll 여섯 장이다.
# nupkg 는 api.nuget.org 에서 받아 %LOCALAPPDATA%\DataTool\nupkg\ 에 두고, 아래 Sha256(nupkg 통째)과 대조한 뒤에만 푼다.
# 판을 올릴 때는 Version · Sha256 을 같이 고친다. Unsafe 를 빼면 컴파일은 되는데 float 열을 읽을 때 터진다.
@{
    MessagePackVersion = '3.1.9'

    Packages = @(
        @{
            Id      = 'MessagePack'
            Version = '3.1.9'
            Entry   = 'lib/netstandard2.1/MessagePack.dll'
            Target  = 'MessagePack.dll'
            Sha256  = '52438B796A20C947D25D8D306CD0D8A7FDD7357F404D3007BC9640E61E1C128A'
        }
        @{
            Id      = 'MessagePack.Annotations'
            Version = '3.1.9'
            Entry   = 'lib/netstandard2.0/MessagePack.Annotations.dll'
            Target  = 'MessagePack.Annotations.dll'
            Sha256  = '2B41BB26405E5589CB589874F3E71254751CBED0E6B1877093928C1DF739B77E'
        }
        @{
            Id      = 'Microsoft.NET.StringTools'
            Version = '17.11.4'
            Entry   = 'lib/netstandard2.0/Microsoft.NET.StringTools.dll'
            Target  = 'Microsoft.NET.StringTools.dll'
            Sha256  = '9567F3637E5643E88A4BD4E9BB3B434E58E017D088391845845126D29D5D5411'
        }
        @{
            Id      = 'System.Collections.Immutable'
            Version = '8.0.0'
            Entry   = 'lib/netstandard2.0/System.Collections.Immutable.dll'
            Target  = 'System.Collections.Immutable.dll'
            Sha256  = '17B3958CA370A6A6D487C95389D6EA256622E3BEA7B2AF67FBA934F90551A37C'
        }
        @{
            Id      = 'System.Runtime.CompilerServices.Unsafe'
            Version = '6.0.0'
            Entry   = 'lib/netstandard2.0/System.Runtime.CompilerServices.Unsafe.dll'
            Target  = 'System.Runtime.CompilerServices.Unsafe.dll'
            Sha256  = '6C41B53E70E9EEE298CFF3A02CE5ACDD15B04125589BE0273F0566026720A762'
        }
        @{
            # 소스 제너레이터(분석기). .meta 를 같이 넣어야 Unity 가 RoslynAnalyzer 로 가져온다.
            Id      = 'MessagePackAnalyzer'
            Version = '3.1.9'
            Entry   = 'analyzers/roslyn4.3/cs/MessagePack.SourceGenerator.dll'
            Target  = 'Analyzers/MessagePack.SourceGenerator.dll'
            Meta    = 'MessagePack.SourceGenerator.dll.meta'
            Sha256  = '3FFFAD66E950E6833A3BD1658B6A40938DC4E58983A5A545D278AF1EE34BEC14'
        }
    )
}
