# Checks the Windows install, one PASS or FAIL line each for the release lanes.
# -Absent checks removal; -DistroDeleted also checks the distro was deleted.
# -Distro supplies its name after uninstall removes the config.
param([switch] $Absent, [switch] $DistroDeleted, [string] $Distro)
$ErrorActionPreference = 'SilentlyContinue'
$script:fails = 0
$script:passes = 0

function Check([string] $name, [bool] $present) {
	$ok = if ($Absent) { -not $present } else { $present }
	if ($ok) { $script:passes++; "PASS $name" } else { $script:fails++; "FAIL $name" }
}

$wsl = 'C:\Program Files\WSL\wsl.exe'
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
# This install's own CA, by thumbprint: a guest that held other distros keeps
# their mkcert CAs, which this uninstall rightly leaves alone.
$caDistro = if ($Distro) { $Distro } else { (Get-Content (Join-Path $lerdBin 'lerd-wsl.conf') | Where-Object { $_ -like 'distro=*' }) -replace '^distro=', '' }
$caPem = if ($caDistro) { (& $wsl -d $caDistro -- sh -c 'cat ~/.local/share/mkcert/rootCA.pem') -join "`n" }
$caThumb = if ($caPem -match 'BEGIN CERTIFICATE') { ([System.Security.Cryptography.X509Certificates.X509Certificate2]::new([System.Text.Encoding]::ASCII.GetBytes($caPem))).Thumbprint }
$trusted = Get-ChildItem Cert:\CurrentUser\Root | Where-Object { if ($caThumb) { $_.Thumbprint -eq $caThumb } else { $_.Subject -match 'mkcert' } }
Check 'W.10 lerd CA trusted by Windows' ($null -ne $trusted)
$distros = (& $wsl -l -q) -replace "`0", '' | Where-Object { $_ -match '\S' }
# The installer keeps a distro from an earlier run, so ask rather than assume.
if (-not $Distro) { $Distro = Get-Content (Join-Path $lerdBin 'lerd-wsl.conf') | Where-Object { $_ -like 'distro=*' } | ForEach-Object { $_.Substring(7) } }
if (-not $Absent -or $DistroDeleted) { Check "W.11 $distro installed" ($distros -contains $distro) }
if (-not $Absent) {
	$dash = try { (Invoke-WebRequest -UseBasicParsing -TimeoutSec 10 http://127.0.0.1:7073/).StatusCode } catch { 0 }
	Check 'W.12 dashboard answers from Windows' ($dash -eq 200)
	$ver = (& (Join-Path $lerdBin 'lerd.exe') --version 2>&1) -join ' '
	Check 'W.13 lerd.exe reaches lerd in WSL' ($ver -match '\d+\.\d+')
}
"SUMMARY windows$(if ($Absent) { ' after uninstall' }): $script:passes passed, $script:fails failed"
