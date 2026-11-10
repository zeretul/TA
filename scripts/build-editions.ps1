param(
    [ValidateSet('local','share')][string]$Edition = 'local',
    [string]$OutputDirectory = ''
)
$ErrorActionPreference = 'Stop'
# Windows PowerShell 5.1 has no utf8NoBOM encoding name, so write UTF-8
# without a byte order mark through .NET directly.
function Write-Utf8NoBom([string]$Path, [string]$Text) {
    [System.IO.File]::WriteAllText($Path, $Text, (New-Object System.Text.UTF8Encoding $false))
}
$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')).TrimEnd('\')
$package = Get-Content -LiteralPath (Join-Path $projectRoot 'web\package.json') -Raw | ConvertFrom-Json
$releaseVersion = [string]$package.version
if($releaseVersion -notmatch '^\d+\.\d+\.\d+$'){ throw 'Version must use digits only.' }
$baselineRoot = Join-Path $projectRoot 'tools\release\share-baseline'
$manifest = Get-Content -LiteralPath (Join-Path $baselineRoot 'manifest.json') -Raw | ConvertFrom-Json
if($OutputDirectory -eq ''){ $OutputDirectory=Join-Path $projectRoot ".ta-dev\edition-build\$Edition" }
$OutputDirectory=[System.IO.Path]::GetFullPath($OutputDirectory)
if(-not $OutputDirectory.StartsWith($projectRoot+'\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Build output must stay in this development directory.'}
$null=New-Item -ItemType Directory -Path $OutputDirectory -Force
$buildArgs=@('build','-tags','embedweb','-ldflags',"-X ta/internal/buildinfo.Version=$releaseVersion",'-o',(Join-Path $OutputDirectory 'TA.exe'))
if($Edition -eq 'share'){
    $replace=@{}
    foreach($entry in $manifest.overrides){
        $sourcePath=Join-Path $projectRoot $entry.source
        $cleanPath=Join-Path $baselineRoot $entry.clean
        $actual=(Get-FileHash -LiteralPath $sourcePath -Algorithm SHA256).Hash.ToLowerInvariant()
        $cleanActual=(Get-FileHash -LiteralPath $cleanPath -Algorithm SHA256).Hash.ToLowerInvariant()
        if($actual -ne $entry.local_sha256 -or $cleanActual -ne $entry.clean_sha256){
            throw "Variant changed without review: $($entry.source). Merge common fixes into the clean baseline and update its manifest first."
        }
        $replace[$sourcePath]=$cleanPath
    }
    $overlayPath=Join-Path $OutputDirectory 'share-overlay.json'
    Write-Utf8NoBom $overlayPath (@{Replace=$replace}|ConvertTo-Json -Depth 5)
    $buildArgs=@('build','-overlay',$overlayPath)+$buildArgs[1..($buildArgs.Count-1)]
}
# Caches may live outside the checkout in TA_DEV_TOOLS, which groups the
# toolchain, caches and temp files for this machine. Without it they stay
# in this checkout's .ta-dev.
if($env:TA_DEV_TOOLS){
    $cacheRoot=Join-Path $env:TA_DEV_TOOLS 'cache'
}else{
    $cacheRoot=Join-Path $projectRoot '.ta-dev'
}
$env:GOCACHE=Join-Path $cacheRoot 'go-build'
$env:GOMODCACHE=Join-Path $cacheRoot 'go-mod'
$env:GOTMPDIR=Join-Path $OutputDirectory 'tmp'
$null=New-Item -ItemType Directory -Path $env:GOTMPDIR -Force
Push-Location -LiteralPath $projectRoot
try {
    & (Join-Path $projectRoot '.ta-toolchain\go\bin\go.exe') @buildArgs ./cmd/ta
    if($LASTEXITCODE -ne 0){throw "Build failed: $Edition"}
    $hash=(Get-FileHash -LiteralPath (Join-Path $OutputDirectory 'TA.exe') -Algorithm SHA256).Hash.ToLowerInvariant()
    Write-Utf8NoBom (Join-Path $OutputDirectory 'build.json') ([pscustomobject]@{version=$releaseVersion;edition=$Edition;sha256=$hash}|ConvertTo-Json)
    "Built $Edition $releaseVersion SHA256=$hash"
} finally { Pop-Location }

