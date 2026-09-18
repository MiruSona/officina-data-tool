# datatool 빌드 스크립트 (Windows PowerShell 5.1)
#
#   .\build.ps1            bin\datatool.exe 를 만든다
#   .\build.ps1 -Test      만들기 전에 go vet · go test 까지 돌린다
#
# CGO 는 끈다 — C 컴파일러 없이 빌드되는 것이 이 툴의 전제다.

[CmdletBinding()]
param(
    [switch]$Test
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path

# go 는 **지금 폴더**의 모듈을 본다. 다른 Go 모듈 안에서 이 스크립트를 부르면
# outside main module 로 죽으므로 제 폴더로 옮겨 간다.
Push-Location $root
try {
    $go = Get-Command go -ErrorAction SilentlyContinue
    if ($null -eq $go) {
        Write-Host "go 를 찾을 수 없다. https://go.dev/dl/ 에서 Go 1.26 이상을 깔고 새 터미널을 연다." -ForegroundColor Red
        exit 1
    }
    Write-Host (& go version)

    $env:CGO_ENABLED = '0'

    if ($Test) {
        & go vet ./...
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        & go test ./...
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }

    $exe = Join-Path $root 'bin\datatool.exe'
    $stamp = (Get-Date).ToString('yyyy-MM-ddTHH:mm:ssK')
    $ldflags = "-s -w -X main.buildTime=$stamp"
    Write-Host "빌드 : $exe"
    # -ldflags 와 값을 한 토큰으로 붙이면 PowerShell 5.1 이 변수를 안 푼다. 따로 넘긴다.
    & go build -trimpath '-ldflags' $ldflags -o $exe (Join-Path $root 'cmd\datatool')
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    $size = [math]::Round((Get-Item $exe).Length / 1MB, 1)
    Write-Host "됐다. $exe ($size MB)" -ForegroundColor Green
    exit 0
}
finally { Pop-Location }
