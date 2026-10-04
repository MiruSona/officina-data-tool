# DataTool Unity 검증 (Windows PowerShell 5.1) — U1~U5 · U4b 를 사람 대신 배치모드로 판정한다.
#
#   .\verify.ps1 -Project <Unity 프로젝트>                       다 돈다
#   .\verify.ps1 -Project <P> -SkipBuild -SkipPlayer              EditMode(U1~U4b)만
#   .\verify.ps1 -Project <P> -SkipTests -SkipBuild -SkipPlayer   dll · 생성 코드만 넣는다
#   .\verify.ps1 -Project <P> -Clean                              Verify 몫(이름에 DataToolVerify)만 지운다
#   .\verify.ps1 -Project <P> -EditorTimeoutMin 60                에디터 한 번 실행의 상한(기본 30분)
#   .\verify.ps1 -Project <P> -Data <데이터 폴더>                  gen·export 를 Testdata\table\ok 대신 이 폴더로 (그 사본만 — 행·해시가 같아야 U4 가 맞는다)
#
# **빈 시험 프로젝트나 사본에서만 돌린다.** Generated · StreamingAssets · Plugins/MessagePack 을 덮어쓴다.
# 그 자리에 우리 것이 아닌 파일(.cs · 다른 판 dll)이 보이면 아무것도 쓰기 전에 멈춘다(종료 2).
# 종료 코드 : 0 통과 · 1 판정 실패(코드를 고칠 일) · 2 환경 문제(기계를 고칠 일 · 잡지 않은 예외 · 시간 넘김).
# 설계 : DataTool/Docs/Design/2026-10-03-Unity검증과웹개선설계.md 3장. 이 파일은 UTF-8 BOM 으로 저장한다.

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Project,
    [string]$UnityExe,
    [string]$DataTool,
    [string]$LogDir,
    [string]$Data,
    [ValidateSet('Disabled', 'Minimal', 'Low', 'Medium', 'High')][string]$Stripping,
    [switch]$SkipToolBuild,
    [switch]$SkipPackages,
    [switch]$SkipData,
    [switch]$SkipCopy,
    [switch]$SkipTests,
    [switch]$SkipBuild,
    [switch]$SkipPlayer,
    [switch]$Clean,
    [ValidateRange(1, 1440)][int]$EditorTimeoutMin = 30
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

# 잡지 않은 예외(네트워크 · zip · Copy-Item 등)는 코드 판정이 아니라 환경 문제다 — 종료 2.
trap {
    Write-Host "환경 문제 — 잡지 않은 예외 : $_" -ForegroundColor Red
    if ($_.InvocationInfo) { Write-Host $_.InvocationInfo.PositionMessage -ForegroundColor DarkGray }
    if ($script:steps -and $script:steps.Count -gt 0) {
        $script:steps | Format-Table -AutoSize | Out-String -Width 200 | Write-Host
    }
    exit 2
}

$verifyDir = $PSScriptRoot
$toolRoot = (Resolve-Path (Join-Path $verifyDir '..\..')).Path
$PlayerTimeoutSec = 60
$EditorTimeoutSec = $EditorTimeoutMin * 60
$GeneratedRel = 'Assets\_Project\Scripts\Data\Generated'
# gen·export 할 데이터. 기본은 시험 골든이다. -Data 는 enum 숫자 고정 같은 꼴만 바꾼 사본을 볼 때 쓴다.
if ($Data) {
    if (-not (Test-Path (Join-Path $Data 'schema.json'))) { Write-Host "환경 문제 — -Data 에 schema.json 이 없다 : $Data" -ForegroundColor Red; exit 2 }
    $okData = (Resolve-Path $Data).Path
}
else {
    $okData = Join-Path $toolRoot 'Testdata\table\ok'
}

# 단계 기록 — 끝의 판정 표에 찍는다.
$script:steps = New-Object System.Collections.ArrayList

function Add-Step([string]$name, [string]$result, $code, [TimeSpan]$took) {
    $sec = '{0:N0}초' -f $took.TotalSeconds
    [void]$script:steps.Add([pscustomobject]@{ 단계 = $name; 결과 = $result; 종료코드 = $code; 시간 = $sec })
}

# 환경 문제로 멈춘다. 지금까지의 단계 표를 찍고 종료 2.
function Stop-Env([string]$message) {
    Write-Host $message -ForegroundColor Red
    if ($script:steps.Count -gt 0) {
        $script:steps | Format-Table -AutoSize | Out-String -Width 200 | Write-Host
    }
    exit 2
}

# GUI 프로그램(Unity · 플레이어)을 띄우고 끝날 때까지 기다린다. 시간 넘김이면 자식까지 통째로 죽이고 -1.
# Start-Process -Wait 는 자식 프로세스까지 기다려 멈출 수 있어 WaitForExit 를 쓴다.
# Unity 는 셰이더 컴파일러 · bee 같은 자식을 띄우므로 Kill() 대신 taskkill /T /F 로 나무째 끈다.
function Invoke-Gui([string]$exe, [string[]]$argList, [int]$timeoutSec) {
    $quoted = ($argList | ForEach-Object { if ($_ -match '\s') { '"' + $_ + '"' } else { $_ } }) -join ' '
    Write-Host "  > $exe $quoted" -ForegroundColor DarkGray
    $p = Start-Process -FilePath $exe -ArgumentList $quoted -PassThru -WindowStyle Hidden
    $null = $p.Handle   # 핸들을 일찍 잡아 둬야 ExitCode 가 비지 않는다 (PS 5.1).
    if ($p.WaitForExit($timeoutSec * 1000) -eq $false) {
        Write-Host "  ${timeoutSec}초가 넘어 프로세스 나무째 끈다 (PID $($p.Id))" -ForegroundColor Yellow
        & taskkill.exe /T /F /PID $p.Id | Out-Null
        $null = $p.WaitForExit(10000)
        return -1
    }
    return $p.ExitCode
}

# 우리가 덮어쓸 자리에 남의 파일이 있으면 멈춘다. 게임 프로젝트를 실수로 지정한 경우를 막는 안전장치다.
function Stop-Foreign([string]$what, [string[]]$items) {
    $list = ($items | Select-Object -First 10) -join ', '
    Stop-Env "$what : $list`n빈 시험 프로젝트나 사본에서만 돌려라 — 이 스크립트는 Generated · StreamingAssets · Plugins/MessagePack 을 덮어쓴다."
}

function Copy-IfChanged([string]$from, [string]$to) {
    $dir = Split-Path -Parent $to
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Force $dir | Out-Null }
    if (Test-Path $to) {
        if ((Get-FileHash $from).Hash -eq (Get-FileHash $to).Hash) { return }
    }
    Copy-Item $from $to -Force
}

# ---- 0. 사전 검사 ---------------------------------------------------------
$t0 = Get-Date
if (-not (Test-Path $Project)) { Stop-Env "프로젝트가 없다 : $Project" }
$Project = (Resolve-Path $Project).Path
$assets = Join-Path $Project 'Assets'

if (Test-Path (Join-Path $Project 'Temp\UnityLockfile')) {
    Stop-Env '프로젝트가 에디터에 열려 있다 — 닫고 다시 (Temp\UnityLockfile)'
}

$verifyPlaces = @(
    (Join-Path $assets 'Tests\DataToolVerify'),
    (Join-Path $assets 'Editor\DataToolVerify'),
    (Join-Path $assets 'DataToolVerify')
)

if ($Clean) {
    foreach ($place in $verifyPlaces) {
        foreach ($item in @($place, "$place.meta")) {
            if (Test-Path $item) { Remove-Item -Recurse -Force $item; Write-Host "지웠다 : $item" }
        }
    }
    Write-Host 'Verify 몫만 지웠다. Plugins · Generated · StreamingAssets 는 게임이 쓰는 자리라 그대로 둔다.'
    exit 0
}

if ($UnityExe -eq '') {
    $versionFile = Join-Path $Project 'ProjectSettings\ProjectVersion.txt'
    if (-not (Test-Path $versionFile)) { Stop-Env "Unity 프로젝트가 아니다 (ProjectVersion.txt 없음) : $Project" }
    $editorVersion = ''
    foreach ($line in Get-Content $versionFile) {
        if ($line -match '^m_EditorVersion:\s*(\S+)') { $editorVersion = $Matches[1]; break }
    }
    if ($editorVersion -eq '') { Stop-Env "ProjectVersion.txt 에 m_EditorVersion 이 없다 : $versionFile" }
    $UnityExe = "C:\Program Files\Unity\Hub\Editor\$editorVersion\Editor\Unity.exe"
    if (-not (Test-Path $UnityExe)) { Stop-Env "에디터 $editorVersion 이 없다 — -UnityExe 로 준다" }
}
elseif (-not (Test-Path $UnityExe)) { Stop-Env "에디터가 없다 : $UnityExe" }

if (-not $SkipTests) {
    $manifestPath = Join-Path $Project 'Packages\manifest.json'
    if (-not (Test-Path $manifestPath)) { Stop-Env "Packages\manifest.json 이 없다 : $manifestPath" }
    $manifest = Get-Content -Raw $manifestPath
    if ($manifest -notmatch '"com\.unity\.test-framework"') {
        Stop-Env 'Packages/manifest.json 에 com.unity.test-framework 가 없다 — Package Manager 로 넣고 다시'
    }
}

if ($LogDir -eq '') { $LogDir = Join-Path $env:TEMP ('datatool-verify-' + (Get-Date).ToString('yyyyMMdd-HHmmss')) }
New-Item -ItemType Directory -Force $LogDir | Out-Null
$LogDir = (Resolve-Path $LogDir).Path
Write-Host "에디터 : $UnityExe"
Write-Host "로그   : $LogDir"
Add-Step '0 사전 검사' '통과' 0 ((Get-Date) - $t0)

# ---- 1. 툴 굽기 -----------------------------------------------------------
$t0 = Get-Date
if ($DataTool -ne '') {
    if (-not (Test-Path $DataTool)) { Stop-Env "datatool 이 없다 : $DataTool" }
    Add-Step '1 툴 굽기' '건너뜀 (-DataTool)' '' ((Get-Date) - $t0)
}
elseif ($SkipToolBuild) {
    $DataTool = Join-Path $toolRoot 'bin\datatool.exe'
    Add-Step '1 툴 굽기' '건너뜀' '' ((Get-Date) - $t0)
}
else {
    $DataTool = Join-Path $toolRoot 'bin\datatool.exe'
    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $toolRoot 'build.ps1')
    $code = $LASTEXITCODE
    if ($code -ne 0) { Add-Step '1 툴 굽기' '실패' $code ((Get-Date) - $t0); Stop-Env 'datatool 을 못 구웠다' }
    Add-Step '1 툴 굽기' '통과' 0 ((Get-Date) - $t0)
}
if (-not (Test-Path $DataTool)) { Stop-Env "datatool 이 없다 : $DataTool" }
$DataTool = (Resolve-Path $DataTool).Path
$dtVersion = (& $DataTool version | Out-String).Trim()
if ($LASTEXITCODE -ne 0) { Stop-Env "datatool version 이 안 돈다 : $DataTool" }
Write-Host "datatool : $dtVersion ($DataTool)"

# 남이 준 exe(-DataTool · -SkipToolBuild)가 지금 소스보다 낡았으면 경고만 한다. 멈추지는 않는다.
# 비교 기준은 build.ps1 이 커밋 칸에 넣는 것과 같다 — 소스(cmd · internal · go.mod)를 마지막으로 바꾼 커밋.
if (($PSBoundParameters.ContainsKey('DataTool')) -or $SkipToolBuild) {
    $exeCommit = ''
    if ($dtVersion -match '커밋 ([0-9a-f]+)') { $exeCommit = $Matches[1] }
    $srcCommit = ''
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $srcCommit = (& git -C $toolRoot log -1 --format=%h -- cmd internal go.mod | Out-String).Trim()
        if ($LASTEXITCODE -ne 0) { $srcCommit = '' }
    }
    if ($exeCommit -ne '' -and $srcCommit -ne '' -and -not ($exeCommit.StartsWith($srcCommit) -or $srcCommit.StartsWith($exeCommit))) {
        Write-Host "경고 : datatool 커밋 $exeCommit 이 지금 소스 커밋 $srcCommit 과 다르다 — 낡은 exe 일 수 있다" -ForegroundColor Yellow
    }
}

# ---- 1b. 안전장치 — Generated 에 남의 .cs 가 있으면 아무것도 쓰기 전에 멈춘다 ----
$genDir = Join-Path $Project $GeneratedRel
if (Test-Path $genDir) {
    $previewDir = Join-Path $LogDir 'gen-preview'
    if (Test-Path $previewDir) { Remove-Item -Recurse -Force $previewDir }
    & $DataTool gen --data $okData --out $previewDir | Out-Null
    if ($LASTEXITCODE -ne 0) { Stop-Env "안전장치용 gen 미리보기가 실패했다 — datatool gen 을 직접 돌려 본다" }
    $ours = @{}
    foreach ($f in Get-ChildItem -File -Recurse -Filter '*.cs' $previewDir) { $ours[$f.Name.ToLowerInvariant()] = $true }
    $foreign = @()
    foreach ($f in Get-ChildItem -File -Recurse -Filter '*.cs' $genDir) {
        if (-not $ours.ContainsKey($f.Name.ToLowerInvariant()) -or $f.DirectoryName -ne $genDir) { $foreign += $f.FullName.Substring($genDir.Length + 1) }
    }
    if ($foreign.Count -gt 0) { Stop-Foreign "Generated 에 우리가 만들지 않은 .cs 가 있다" $foreign }
}

# ---- 2. 패키지 ------------------------------------------------------------
$t0 = Get-Date
$pluginDir = Join-Path $assets 'Plugins\MessagePack'
$list = Import-PowerShellDataFile (Join-Path $verifyDir 'Packages.psd1')
$ourDlls = @{}
foreach ($pkg in $list.Packages) { $ourDlls[($pkg.Target -replace '/', '\').ToLowerInvariant()] = $pkg }

# 안전장치 ① — Plugins/MessagePack 에 우리 목록 밖의 dll 이 있으면 멈춘다 (-SkipPackages 여도 본다).
$existingDlls = @()
if (Test-Path $pluginDir) { $existingDlls = @(Get-ChildItem -File -Recurse -Filter '*.dll' $pluginDir) }
$unknown = @($existingDlls | Where-Object { -not $ourDlls.ContainsKey($_.FullName.Substring($pluginDir.Length + 1).ToLowerInvariant()) } |
    ForEach-Object { $_.FullName.Substring($pluginDir.Length + 1) })
if ($unknown.Count -gt 0) { Stop-Foreign "Plugins/MessagePack 에 Packages.psd1 에 없는 dll 이 있다" $unknown }

if ($SkipPackages) {
    Add-Step '2 패키지' '건너뜀' '' ((Get-Date) - $t0)
}
else {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $cache = Join-Path $env:LOCALAPPDATA 'DataTool\nupkg'
    New-Item -ItemType Directory -Force $cache | Out-Null
    $stage = Join-Path $LogDir 'pkg'
    New-Item -ItemType Directory -Force $stage | Out-Null

    # 먼저 전부 받아 풀어 둔다. 복사는 안전장치 ② 를 지난 뒤에 한다.
    $staged = @()
    foreach ($pkg in $list.Packages) {
        $id = $pkg.Id.ToLowerInvariant()
        $nupkg = Join-Path $cache "$id.$($pkg.Version).nupkg"
        if (-not (Test-Path $nupkg)) {
            $url = "https://api.nuget.org/v3-flatcontainer/$id/$($pkg.Version)/$id.$($pkg.Version).nupkg"
            Write-Host "  받는 중 : $url"
            Invoke-WebRequest -Uri $url -OutFile "$nupkg.part" -UseBasicParsing
            Move-Item -Force "$nupkg.part" $nupkg
        }
        if ((Get-FileHash $nupkg -Algorithm SHA256).Hash -ne $pkg.Sha256) {
            Add-Step '2 패키지' '실패' 2 ((Get-Date) - $t0)
            Stop-Env "$($pkg.Id) 해시가 다르다 — 받은 파일을 지우고 다시 : $nupkg"
        }

        # nupkg 안 경로에 // 가 섞인 것이 있어(분석기) 겹친 / 를 하나로 보고 찾는다.
        $zip = [System.IO.Compression.ZipFile]::OpenRead($nupkg)
        try {
            $entry = $zip.Entries | Where-Object { ($_.FullName -replace '/+', '/') -eq $pkg.Entry } | Select-Object -First 1
            if ($null -eq $entry) { Stop-Env "$($pkg.Id) 안에 $($pkg.Entry) 가 없다" }
            $tmp = Join-Path $stage ([IO.Path]::GetFileName($pkg.Target))
            [System.IO.Compression.ZipFileExtensions]::ExtractToFile($entry, $tmp, $true)
        }
        finally { $zip.Dispose() }
        $staged += [pscustomobject]@{ Pkg = $pkg; Tmp = $tmp }
    }

    # 안전장치 ② — 같은 이름인데 내용이 다른 dll(다른 판)이 이미 있으면 덮지 않고 멈춘다.
    $otherVersion = @()
    foreach ($s in $staged) {
        $dest = Join-Path $pluginDir $s.Pkg.Target
        if ((Test-Path $dest) -and ((Get-FileHash $dest).Hash -ne (Get-FileHash $s.Tmp).Hash)) {
            $otherVersion += "$($s.Pkg.Target) (기대 판 $($s.Pkg.Version))"
        }
    }
    if ($otherVersion.Count -gt 0) { Stop-Foreign "Plugins/MessagePack 에 Packages.psd1 과 다른 판 dll 이 있다" $otherVersion }

    foreach ($s in $staged) {
        $pkg = $s.Pkg
        Copy-IfChanged $s.Tmp (Join-Path $pluginDir $pkg.Target)
        if ($pkg.ContainsKey('Meta')) {
            Copy-IfChanged (Join-Path $verifyDir "Meta\$($pkg.Meta)") (Join-Path $pluginDir "$($pkg.Target).meta")
        }
        Remove-Item $s.Tmp
        Write-Host "  $($pkg.Id) $($pkg.Version) → Plugins/MessagePack/$($pkg.Target)"
    }
    Add-Step '2 패키지' '통과' 0 ((Get-Date) - $t0)
}

# ---- 3. 데이터 ------------------------------------------------------------
$t0 = Get-Date
if ($SkipData) {
    Add-Step '3 데이터' '건너뜀' '' ((Get-Date) - $t0)
}
else {
    $u4bData = Join-Path $verifyDir 'Fixtures\u4b'
    $testDir = Join-Path $assets 'Tests\DataToolVerify'
    New-Item -ItemType Directory -Force (Join-Path $assets 'StreamingAssets'), $testDir | Out-Null

    $runs = @(
        @('gen', '--data', $okData, '--out', (Join-Path $assets '_Project\Scripts\Data\Generated')),
        @('export', '--data', $okData, '--out', (Join-Path $assets 'StreamingAssets\gamedata.bytes')),
        @('export', '--data', $u4bData, '--out', (Join-Path $testDir 'gamedata-u4b.bytes'))
    )
    foreach ($run in $runs) {
        & $DataTool @run
        if ($LASTEXITCODE -ne 0) {
            Add-Step '3 데이터' '실패' $LASTEXITCODE ((Get-Date) - $t0)
            Stop-Env "gen/export 실패 — 위 출력 ($($run[0]) $($run[2]))"
        }
    }
    Add-Step '3 데이터' '통과' 0 ((Get-Date) - $t0)
}

# ---- 4. 코드 복사 ---------------------------------------------------------
$t0 = Get-Date
if ($SkipCopy) {
    Add-Step '4 코드 복사' '건너뜀' '' ((Get-Date) - $t0)
}
else {
    $copies = @(
        @{ From = 'Tests'; To = $verifyPlaces[0] },
        @{ From = 'Editor'; To = $verifyPlaces[1] },
        @{ From = 'Runtime'; To = $verifyPlaces[2] }
    )
    foreach ($c in $copies) {
        foreach ($f in Get-ChildItem -File (Join-Path $verifyDir $c.From)) {
            Copy-IfChanged $f.FullName (Join-Path $c.To $f.Name)
        }
    }
    Add-Step '4 코드 복사' '통과' 0 ((Get-Date) - $t0)
}

# ---- 5. EditMode ---------------------------------------------------------
$verdict = [ordered]@{ U1 = '건너뜀'; 'U2·U3' = '건너뜀'; U4 = '건너뜀'; U4b = '건너뜀'; U5 = '건너뜀' }
$why = [ordered]@{}
$caseOf = [ordered]@{
    U1      = 'U1_SourceGeneratorMadeFormatters'
    'U2·U3' = 'U2_U3_LoadBakedAndCompareValues'
    U4      = 'U4_SchemaHashMismatchThrows'
    U4b     = 'U4b_OldBakeAfterColumnTypeChangeThrows'
}

$t0 = Get-Date
if ($SkipTests) {
    Add-Step '5 EditMode' '건너뜀' '' ((Get-Date) - $t0)
}
else {
    $xmlPath = Join-Path $LogDir 'editmode.xml'
    $logPath = Join-Path $LogDir 'editmode.log'
    if (Test-Path $xmlPath) { Remove-Item $xmlPath }
    # Verify 시험 asmdef 하나만 돌린다. 프로젝트에 다른 시험이 있어도 판정과 시간이 섞이지 않게.
    $code = Invoke-Gui $UnityExe @('-batchmode', '-nographics', '-projectPath', $Project, '-runTests', '-testPlatform', 'EditMode',
        '-assemblyNames', 'DataToolVerify.Tests', '-testResults', $xmlPath, '-logFile', $logPath) $EditorTimeoutSec
    if ($code -eq -1) {
        Add-Step '5 EditMode' '시간 넘김' $code ((Get-Date) - $t0)
        Stop-Env "에디터(EditMode)가 ${EditorTimeoutMin}분 안에 안 끝나 껐다 — $logPath 를 본다 (-EditorTimeoutMin 으로 늘린다)"
    }

    $csErrors = @()
    if (Test-Path $logPath) { $csErrors = @(Select-String -Path $logPath -Pattern 'error CS\d+' | Select-Object -First 5) }
    if (-not (Test-Path $xmlPath) -and $csErrors.Count -eq 0) {
        Add-Step '5 EditMode' '결과 없음' $code ((Get-Date) - $t0)
        Stop-Env "환경 문제 — 시험 결과 XML 도 컴파일 오류도 없다 (라이선스 · 에디터 실행 문제?) — $logPath 를 본다"
    }
    $cases = @{}
    if (Test-Path $xmlPath) {
        $xml = [xml](Get-Content -Raw -Encoding UTF8 $xmlPath)
        foreach ($tc in $xml.SelectNodes('//test-case')) { $cases[$tc.name] = $tc }
    }

    foreach ($u in $caseOf.Keys) {
        $tc = $cases[$caseOf[$u]]
        if ($null -eq $tc) { $verdict[$u] = '실패'; $why[$u] = '시험 결과에 없다 (컴파일 실패?)'; continue }
        if ($tc.result -eq 'Passed') { $verdict[$u] = '통과'; continue }
        $verdict[$u] = '실패'
        $msg = $tc.SelectSingleNode('failure/message')
        $why[$u] = '시험 실패'
        if ($null -ne $msg) { $why[$u] = ($msg.InnerText -replace '\s+', ' ').Trim() }
    }
    if ($csErrors.Count -gt 0) {
        $verdict['U1'] = '실패'
        $why['U1'] = '컴파일 오류 : ' + (($csErrors | ForEach-Object { $_.Line.Trim() }) -join ' | ')
    }

    $result = '통과'
    if ($code -ne 0 -or ($verdict.Values -contains '실패')) { $result = '실패' }
    Add-Step '5 EditMode' $result $code ((Get-Date) - $t0)
}

# ---- 6. IL2CPP 빌드 · 7. 플레이어 -----------------------------------------
$playerDir = Join-Path $LogDir 'player'
$buildOk = $false
$t0 = Get-Date
if ($SkipBuild) {
    Add-Step '6 IL2CPP 빌드' '건너뜀' '' ((Get-Date) - $t0)
}
else {
    $argList = @('-batchmode', '-nographics', '-projectPath', $Project, '-executeMethod', 'DataToolVerifyBuild.BuildWin64Il2Cpp',
        '-buildOut', $playerDir, '-logFile', (Join-Path $LogDir 'build.log'))
    if ($Stripping -ne '') { $argList += @('-stripping', $Stripping) }
    $code = Invoke-Gui $UnityExe $argList $EditorTimeoutSec
    if ($code -eq -1) {
        Add-Step '6 IL2CPP 빌드' '시간 넘김' $code ((Get-Date) - $t0)
        Stop-Env "에디터(빌드)가 ${EditorTimeoutMin}분 안에 안 끝나 껐다 — ProjectSettings 가 IL2CPP 로 남았을 수 있다. build.log 를 본다"
    }
    if ($code -eq 2) {
        Add-Step '6 IL2CPP 빌드' '환경 문제' $code ((Get-Date) - $t0)
        Stop-Env "빌드 진입점이 환경 문제로 끝났다 (인자 · 예외) — build.log 의 'DATATOOL-VERIFY 환경 문제' 줄을 본다"
    }
    $buildOk = ($code -eq 0) -and (Test-Path (Join-Path $playerDir 'GameAssembly.dll'))
    if ($buildOk) {
        Add-Step '6 IL2CPP 빌드' '통과' $code ((Get-Date) - $t0)
    }
    else {
        Add-Step '6 IL2CPP 빌드' '실패' $code ((Get-Date) - $t0)
        $verdict['U5'] = '실패'
        $first = Select-String -Path (Join-Path $LogDir 'build.log') -Pattern 'error' -ErrorAction SilentlyContinue | Select-Object -First 1
        $why['U5'] = 'IL2CPP 빌드 실패 — ' + $(if ($first) { $first.Line.Trim() } else { 'build.log 를 본다' })
    }
}

$t0 = Get-Date
if ($SkipPlayer) {
    Add-Step '7 플레이어' '건너뜀' '' ((Get-Date) - $t0)
}
elseif ($verdict['U5'] -eq '실패') {
    Add-Step '7 플레이어' '못 돎 (빌드 실패)' '' ((Get-Date) - $t0)
}
else {
    $playerExe = Join-Path $playerDir 'Verify.exe'
    $playerLog = Join-Path $LogDir 'player.log'
    if (-not (Test-Path $playerExe)) {
        Add-Step '7 플레이어' '실패' '' ((Get-Date) - $t0)
        $verdict['U5'] = '실패'; $why['U5'] = "플레이어가 없다 — -SkipBuild 면 먼저 빌드한다 : $playerExe"
    }
    else {
        $code = Invoke-Gui $playerExe @('-batchmode', '-nographics', '-datatoolVerify', '-logFile', $playerLog) $PlayerTimeoutSec
        $resultLine = ''
        $il2cpp = $false
        if (Test-Path $playerLog) {
            $hit = Select-String -Path $playerLog -Pattern 'PROBE RESULT' | Select-Object -First 1
            if ($hit) { $resultLine = $hit.Line.Trim() }
            $il2cpp = [bool](Select-String -Path $playerLog -Pattern 'il2cpp=True' -Quiet)
        }
        $ok = ($code -eq 0) -and ($resultLine -eq 'PROBE RESULT OK') -and $il2cpp
        if ($ok) {
            Add-Step '7 플레이어' '통과' $code ((Get-Date) - $t0)
            if ($SkipBuild -or $buildOk) { $verdict['U5'] = '통과' }
        }
        else {
            Add-Step '7 플레이어' '실패' $code ((Get-Date) - $t0)
            $verdict['U5'] = '실패'
            if ($code -eq -1) { $why['U5'] = "플레이어가 ${PlayerTimeoutSec}초 안에 안 끝나 죽였다" }
            else { $why['U5'] = "플레이어 종료 $code — $resultLine (il2cpp=$il2cpp)" }
        }
    }
}

# ---- 8. 판정 --------------------------------------------------------------
Write-Host ''
Write-Host '== 단계 ==' -ForegroundColor Cyan
$script:steps | Format-Table -AutoSize | Out-String -Width 200 | Write-Host
Write-Host '== 판정 ==' -ForegroundColor Cyan
# 까닭은 앞 300자만 찍는다. 전체(스택 포함)는 로그 폴더의 editmode.xml · build.log · player.log 에 있다.
$rows = foreach ($u in $verdict.Keys) {
    $reason = [string]$why[$u]
    if ($reason.Length -gt 300) { $reason = $reason.Substring(0, 300) + ' …' }
    [pscustomobject]@{ U = $u; 판정 = $verdict[$u]; 까닭 = $reason }
}
$rows | Format-Table -AutoSize -Wrap | Out-String -Width 200 | Write-Host
Write-Host "datatool : $dtVersion"
Write-Host "로그     : $LogDir"

# U 판정이 다 통과여도 단계(에디터 종료 코드 등)가 실패면 실패다.
$failedSteps = @($script:steps | Where-Object { $_.결과 -eq '실패' })
if (($verdict.Values -contains '실패') -or $failedSteps.Count -gt 0) {
    if ($failedSteps.Count -gt 0) { Write-Host ('실패한 단계 : ' + (($failedSteps | ForEach-Object { "$($_.단계)(종료 $($_.종료코드))" }) -join ', ')) -ForegroundColor Red }
    Write-Host '판정 실패가 있다.' -ForegroundColor Red
    exit 1
}
Write-Host '실패 없음.' -ForegroundColor Green
exit 0
