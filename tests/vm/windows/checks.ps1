# Checks what the Lerd installer leaves on Windows, one PASS or FAIL line each,
# in the format the release lanes read. -Absent checks the same things are gone
# after an uninstall; the distro goes too only when it was deleted, -DistroDeleted.
param([switch] $Absent, [switch] $DistroDeleted)
$ErrorActionPreference = 'SilentlyContinue'
$script:fails = 0
$script:passes = 0

function Check([string] $name, [bool] $present) {
	$ok = if ($Absent) { -not $present } else { $present }
	if ($ok) { $script:passes++; "PASS $name" } else { $script:fails++; "FAIL $name" }
}

$wsl = 'C:\Program Files\WSL\wsl.exe'
$distro = 'Ubuntu-26.04'
$lerdBin = Join-Path $env:LOCALAPPDATA 'lerd\bin'
$uninstallKey = 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{6F2B7C1E-3D4A-4E8B-9C2F-1A5D8E7B4C30}_is1'

Check 'W.1 Add/Remove Programs entry' (Test-Path $uninstallKey)
Check 'W.2 install folder' (Test-Path 'C:\Program Files\Lerd')
Check 'W.3 desktop app installed' (Test-Path 'C:\Program Files\Lerd\desktop\lerd-desktop.exe')
Check 'W.4 desktop app starts at sign-in' ($null -ne (Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run').'lerd-desktop')
Check 'W.5 desktop app running' ($null -ne (Get-Process -Name 'lerd-desktop' -ErrorAction SilentlyContinue))
Check 'W.6 Start menu shortcut' ($null -ne (Get-ChildItem "$env:APPDATA\Microsoft\Windows\Start Menu\Programs", 'C:\ProgramData\Microsoft\Windows\Start Menu\Programs' -Recurse -Filter 'Lerd*.lnk' | Select-Object -First 1))
Check 'W.7 lerd:// handler' (Test-Path 'HKCU:\Software\Classes\lerd')
Check 'W.8 lerd.exe on the user PATH' (([Environment]::GetEnvironmentVariable('Path', 'User') -split ';') -contains $lerdBin -and (Test-Path (Join-Path $lerdBin 'lerd.exe')))
Check 'W.9 .test resolves through lerd' ($null -ne (Get-DnsClientNrptRule | Where-Object { $_.Comment -eq 'lerd' }))
Check 'W.10 lerd CA trusted by Windows' ($null -ne (Get-ChildItem Cert:\CurrentUser\Root | Where-Object { $_.Subject -match 'mkcert' }))
$distros = (& $wsl -l -q) -replace "`0", '' | Where-Object { $_ -match '\S' }
if (-not $Absent -or $DistroDeleted) { Check "W.11 $distro installed" ($distros -contains $distro) }
if (-not $Absent) {
	$dash = try { (Invoke-WebRequest -UseBasicParsing -TimeoutSec 10 http://127.0.0.1:7073/).StatusCode } catch { 0 }
	Check 'W.12 dashboard answers from Windows' ($dash -eq 200)
	$ver = (& (Join-Path $lerdBin 'lerd.exe') --version 2>&1) -join ' '
	Check 'W.13 lerd.exe reaches lerd in WSL' ($ver -match '\d+\.\d+')
}
"SUMMARY windows$(if ($Absent) { ' after uninstall' }): $script:passes passed, $script:fails failed"
