param([ValidatePattern('^history-[a-z0-9-]+$')][string]$Batch='history-complete',[ValidateRange(1024,65535)][int]$Port=18071)
$ErrorActionPreference='Stop'
$projectRoot=[System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$testRoot=Join-Path $projectRoot ('.ta-dev\'+$Batch)
$runtimeRoot=Join-Path $testRoot 'runtime'
$exePath=Join-Path $runtimeRoot 'TA.exe'
$listener=Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if($listener){
    $running=Get-CimInstance Win32_Process -Filter "ProcessId=$($listener.OwningProcess)"
    if($running.ExecutablePath -ne $exePath){throw "Port $Port belongs to another application."}
    Write-Output "Existing isolated candidate PID=$($running.ProcessId)"
    exit 0
}
if(-not (Test-Path -LiteralPath $exePath)){throw 'Build the isolated candidate first.'}
$env:TA_DIR=Join-Path $testRoot 'data'
$env:DENOVA_DIR=$env:TA_DIR
$env:NOVA_DIR=$env:TA_DIR
Remove-Item Env:TA_WORKSPACE,Env:DENOVA_WORKSPACE,Env:NOVA_WORKSPACE -ErrorAction SilentlyContinue
$process=Start-Process -FilePath $exePath -ArgumentList '--port',([string]$Port),'--no-open' -WorkingDirectory $runtimeRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $testRoot 'candidate.out.log') -RedirectStandardError (Join-Path $testRoot 'candidate.err.log')
$process.Id | Set-Content -LiteralPath (Join-Path $testRoot 'server.pid')
Write-Output "Started isolated candidate PID=$($process.Id)"
