$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$out = Join-Path $root 'tapcanvas-web.out.log'
$err = Join-Path $root 'tapcanvas-web.err.log'
$node = (Get-Command node.exe).Source
$vite = Join-Path $root 'node_modules\vite\bin\vite.js'
$process = Start-Process -FilePath $node -ArgumentList $vite,'preview','--host','0.0.0.0','--port','5175' -WorkingDirectory $root -RedirectStandardOutput $out -RedirectStandardError $err -PassThru -WindowStyle Hidden
$process.Id | Set-Content -LiteralPath (Join-Path $root 'tapcanvas-web.pid')
Write-Output "TapCanvas Web started with PID $($process.Id) on http://127.0.0.1:5175"
