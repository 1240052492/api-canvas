$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$out = Join-Path $root 'tapcanvas-api.out.log'
$err = Join-Path $root 'tapcanvas-api.err.log'
$node = (Get-Command node.exe).Source
$process = Start-Process -FilePath $node -ArgumentList '-r','ts-node/register/transpile-only','-r','tsconfig-paths/register','src/main.ts' -WorkingDirectory $root -RedirectStandardOutput $out -RedirectStandardError $err -PassThru -WindowStyle Hidden
$process.Id | Set-Content -LiteralPath (Join-Path $root 'tapcanvas-api.pid')
Write-Output "TapCanvas API started with PID $($process.Id) on http://127.0.0.1:8788"
