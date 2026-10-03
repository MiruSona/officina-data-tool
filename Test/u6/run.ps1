# U6 자동 판정 — datatool serve 표 편집 UI 를 크롬 headless 로 몰아 git diff 로 판정한다 (Windows PowerShell 5.1)
#
#   .\run.ps1                         소스로 datatool 을 임시 폴더에 새로 굽고 돈다 (bin\ 의 exe 는 안 건드린다)
#   .\run.ps1 -DataTool <exe>         이미 구운 exe 로 돈다
#   .\run.ps1 -Keep                   임시 데이터 폴더·크롬 프로필을 남긴다 (실패 따라가기용)
#   .\run.ps1 -OutDir <폴더>          결과 JSON·그림을 둘 곳 (기본: 임시 폴더 옆 -out)
#   .\run.ps1 -Rows <N>               item·drop 행 수 (기본 2000, 그 아래는 막는다 — 시나리오가 item_1999 등을 집어 쓴다)
#
# 차례 : 굽기 → 임시 폴더에 schema.json + node gen.js → datatool fmt → git init · 첫 커밋 →
#        datatool serve --json → 크롬 --headless=new --remote-debugging-port → node u6.js → 판정 표 → 뒷정리
# 종료 코드 : 0 전부 통과 · 1 판정 실패 · 2 환경 문제(Node·크롬·git·go 없음, 서버·크롬 못 띄움)
# 시나리오 표는 설계 Docs/Design/2026-10-03-Unity검증과웹개선설계.md 7장.
#
# 이 파일은 UTF-8 BOM 으로 저장한다 — PowerShell 5.1 이 BOM 없는 한글을 깨 읽는다.

[CmdletBinding()]
param(
    [string]$DataTool = '',
    [string]$Chrome = '',
    [string]$OutDir = '',
    [int]$Rows = 2000,
    [switch]$Keep
)

$ErrorActionPreference = 'Stop'
# 아래 try 밖(사전 검사 등)에서 예기치 않게 터진 것도 「판정 실패(1)」가 아니라 「환경 문제(2)」로 낸다.
trap {
    Write-Host "환경 문제 : $_" -ForegroundColor Red
    exit 2
}
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$toolRoot = Resolve-Path (Join-Path $here '..\..')

function Stop-Env([string]$message) {
    Write-Host "환경 문제 : $message" -ForegroundColor Red
    exit 2
}

# FreePort 는 빈 TCP 포트 하나를 받는다. 닫은 뒤 크롬이 잡기까지 틈이 있지만 시험 기계에선 충분하다.
function Get-FreePort {
    $listener = New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = $listener.LocalEndpoint.Port
    $listener.Stop()
    return $port
}

# Invoke-Native 는 네이티브 명령을 돌리고 종료 코드를 돌려준다.
# PS 5.1 은 Stop 에서 네이티브 stderr 를 ErrorRecord 로 터뜨리므로 잠깐 Continue 로 둔다.
function Invoke-Native([string]$exe, [string[]]$argList) {
    $saved = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out = & $exe @argList 2>&1 | Out-String
    } finally {
        $ErrorActionPreference = $saved
    }
    return @{ Code = $LASTEXITCODE; Out = $out }
}

# --- 0. 사전 검사 ---------------------------------------------------------
if ($Rows -lt 2000) { Stop-Env "-Rows $Rows 이다 — 2000 이상이어야 한다 (S2·S7 등이 item_1999·item_0800 을 집어 쓴다)" }
$node = Get-Command node -ErrorAction SilentlyContinue
if ($null -eq $node) { Stop-Env 'node 가 없다 — Node 22 이상을 깐다 (내장 WebSocket 을 쓴다)' }
$nodeMajor = [int](((& node --version) -replace '^v', '').Split('.')[0])
if ($nodeMajor -lt 22) { Stop-Env "Node $nodeMajor 이다 — 22 이상이어야 내장 WebSocket 이 있다" }
if ($null -eq (Get-Command git -ErrorAction SilentlyContinue)) { Stop-Env 'git 이 없다' }

if ($Chrome -eq '') {
    $candidates = @(
        "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
        "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
        "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe",
        "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
        "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe"
    )
    $Chrome = $candidates | Where-Object { $_ -and (Test-Path $_) } | Select-Object -First 1
}
if (-not $Chrome -or -not (Test-Path $Chrome)) { Stop-Env '크롬(또는 Edge)을 못 찾았다 — -Chrome <경로> 로 준다' }

$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$work = Join-Path $env:TEMP "datatool-u6-$stamp"
if ($OutDir -eq '') { $OutDir = "$work-out" }
New-Item -ItemType Directory -Force -Path $work, $OutDir | Out-Null
$data = Join-Path $work 'data'
New-Item -ItemType Directory -Force -Path $data | Out-Null

$serveProc = $null
$chromeProc = $null
$exitCode = 2

try {
    # --- 1. 굽기 ------------------------------------------------------------
    if ($DataTool -eq '') {
        if ($null -eq (Get-Command go -ErrorAction SilentlyContinue)) { Stop-Env 'go 가 없다 — -DataTool <exe> 로 구운 것을 준다' }
        $DataTool = Join-Path $work 'datatool.exe'
        Write-Host "굽기 : $DataTool"
        $env:CGO_ENABLED = '0'
        Push-Location $toolRoot
        try { $r = Invoke-Native 'go' @('build', '-o', $DataTool, './cmd/datatool') } finally { Pop-Location }
        if ($r.Code -ne 0) { Write-Host $r.Out; Stop-Env 'datatool 을 못 구웠다' }
    }
    if (-not (Test-Path $DataTool)) { Stop-Env "datatool 이 없다 : $DataTool" }
    $DataTool = (Resolve-Path $DataTool).Path

    # --- 2. 데이터 · git ----------------------------------------------------
    Copy-Item (Join-Path $toolRoot 'Testdata\table\ok\schema.json') $data
    $r = Invoke-Native 'node' @((Join-Path $here 'gen.js'), $data, "$Rows")
    if ($r.Code -ne 0) { Write-Host $r.Out; Stop-Env 'gen.js 가 실패했다' }
    Write-Host $r.Out.Trim()
    $r = Invoke-Native $DataTool @('fmt', '--data', $data)
    if ($r.Code -ne 0) { Write-Host $r.Out; Stop-Env "datatool fmt 가 종료 $($r.Code) 다" }

    # 사용자 전역 훅(core.hooksPath·템플릿 훅)이 시험 커밋에 걸리지 않게 빈 훅 폴더를 가리킨다.
    # 서명(commit.gpgsign) 설정은 건드리지 않는다.
    $noHooks = Join-Path $work 'nohooks'
    New-Item -ItemType Directory -Force -Path $noHooks | Out-Null
    foreach ($g in @(
            @('init', '-q'),
            @('config', 'core.hooksPath', $noHooks),
            @('config', 'core.autocrlf', 'false'),
            @('config', 'user.name', 'u6'),
            @('config', 'user.email', 'u6@localhost'),
            @('add', '-A'),
            @('commit', '-q', '-m', 'U6 기준판'))) {
        $r = Invoke-Native 'git' (@('-C', $data) + $g)
        if ($r.Code -ne 0) { Write-Host $r.Out; Stop-Env "git $($g -join ' ') 실패" }
    }

    # --- 3. serve -----------------------------------------------------------
    $serveOut = Join-Path $work 'serve.out'
    $serveErr = Join-Path $work 'serve.err'
    $serveProc = Start-Process -FilePath $DataTool -ArgumentList @('serve', '--data', "`"$data`"", '--json') `
        -RedirectStandardOutput $serveOut -RedirectStandardError $serveErr -WindowStyle Hidden -PassThru
    $url = ''
    for ($i = 0; $i -lt 100 -and $url -eq ''; $i++) {
        Start-Sleep -Milliseconds 100
        if ($serveProc.HasExited) { break }
        if (Test-Path $serveOut) {
            $line = Get-Content $serveOut -Encoding UTF8 -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($line) { $url = ($line | ConvertFrom-Json).url }
        }
    }
    if ($url -eq '') { Get-Content $serveErr -ErrorAction SilentlyContinue | Write-Host; Stop-Env 'datatool serve 가 주소를 안 냈다' }
    Write-Host "serve : $url (PID $($serveProc.Id))"

    # --- 4. 크롬 ------------------------------------------------------------
    $cdpPort = Get-FreePort
    $chromeProfile = Join-Path $work 'chrome'
    $chromeArgs = @('--headless=new', '--disable-gpu', "--remote-debugging-port=$cdpPort", "--user-data-dir=`"$chromeProfile`"",
        '--no-first-run', '--no-default-browser-check', '--disable-extensions', 'about:blank')
    $chromeProc = Start-Process -FilePath $Chrome -ArgumentList $chromeArgs -WindowStyle Hidden -PassThru
    Write-Host "크롬 : 포트 $cdpPort (PID $($chromeProc.Id))"

    # --- 5. 시나리오 --------------------------------------------------------
    $saved = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    & node (Join-Path $here 'u6.js') --url $url --data $data --cdp $cdpPort --out $OutDir `
        --serve-pid $serveProc.Id --exe $DataTool 2>&1 | Tee-Object -FilePath (Join-Path $OutDir 'u6.log') | Out-Null
    $nodeCode = $LASTEXITCODE
    $ErrorActionPreference = $saved

    # --- 6. 판정 표 ---------------------------------------------------------
    $resultFile = Join-Path $OutDir 'u6-results.json'
    if (-not (Test-Path $resultFile)) {
        Get-Content (Join-Path $OutDir 'u6.log') -Encoding UTF8 | Write-Host
        if ($nodeCode -eq 2) { Stop-Env '크롬에 못 붙었다' }
        Write-Host 'u6.js 가 결과를 안 남겼다' -ForegroundColor Red
        $exitCode = 1
    } else {
        $json = Get-Content $resultFile -Raw -Encoding UTF8 | ConvertFrom-Json
        Write-Host ''
        Write-Host '| # | 시나리오 | 기대 | 실제 numstat | 판정 |'
        Write-Host '| --- | --- | --- | --- | --- |'
        foreach ($res in $json.results) {
            $mark = if ($res.pass) { '통과' } else { '실패' }
            Write-Host "| $($res.id) | $($res.title) | $($res.expect) | $($res.actual) | $mark |"
        }
        $failed = @($json.results | Where-Object { -not $_.pass })
        foreach ($res in $failed) { Write-Host "실패 $($res.id) : $($res.note)" -ForegroundColor Red }
        if ($json.crashed) { Write-Host "도중에 죽음 : $($json.crashed)" -ForegroundColor Red }
        $total = @($json.results).Count
        Write-Host ''
        if ($nodeCode -eq 0 -and $failed.Count -eq 0) {
            Write-Host "U6 : $total/$total 통과" -ForegroundColor Green
            $exitCode = 0
        } else {
            Write-Host "U6 : $($total - $failed.Count)/$total 통과" -ForegroundColor Red
            $exitCode = 1
        }
    }
    Write-Host "결과 : $OutDir"
}
catch {
    # 시나리오 판정이 아니라 스크립트가 터진 것이다 — 뒷정리(finally) 뒤 종료 2.
    Write-Host "환경 문제 : $_" -ForegroundColor Red
    $exitCode = 2
}
finally {
    # 크롬은 자식 프로세스를 여럿 띄운다. 나무째 끈다. 같은 프로필을 쓰는 남은 것도 찾아 끈다.
    if ($chromeProc -and -not $chromeProc.HasExited) {
        & taskkill /PID $chromeProc.Id /T /F 2>&1 | Out-Null
    }
    Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -and $_.CommandLine.Contains("datatool-u6-$stamp") } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
    if ($serveProc -and -not $serveProc.HasExited) { Stop-Process -Id $serveProc.Id -Force -ErrorAction SilentlyContinue }
    if (-not $Keep) {
        Start-Sleep -Milliseconds 500
        Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
    } else {
        Write-Host "남긴 임시 폴더 : $work"
    }
}
exit $exitCode
