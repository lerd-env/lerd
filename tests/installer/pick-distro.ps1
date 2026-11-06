# Tests install.ps1's distro picker, whose functions are lifted out of the script
# since the script itself runs on load. `wsl` output is stubbed through Capture.
$ErrorActionPreference = 'Stop'
$script = Get-Content (Join-Path $PSScriptRoot '..\..\install.ps1') -Raw
$start = $script.IndexOf('  # NewestUbuntu returns')
$end = $script.IndexOf('  # ExistingUser returns')
Invoke-Expression $script.Substring($start, $end - $start)

$FallbackDistro = 'Ubuntu-26.04'
$MinUbuntu = 2404
$RealWSLExe = $PSHOME
$root = Join-Path ([IO.Path]::GetTempPath()) "lerd-pick-$PID"
$env:LOCALAPPDATA = $root
$conf = Join-Path $root 'lerd\bin\lerd-wsl.conf'
$null = New-Item -ItemType Directory -Force (Split-Path $conf)

function WSLInstalled { $true }
function Capture([string]$exe, [string]$params) {
  if ($params -eq '-l -q') { return @{ Code = 0; Lines = @($script:Installed) } }
  @{ Code = $script:ListCode; Lines = @($script:Offered) }
}

$failed = 0
function Case($name, $wired, $installed, $offered, $code, $pick, $ready) {
  Remove-Item $conf -ErrorAction SilentlyContinue
  if ($wired) { "distro=$wired", 'user=me' | Set-Content $conf }
  $script:Installed = $installed; $script:Offered = $offered; $script:ListCode = $code
  $gotPick = PickDistro
  $gotReady = [bool](WSLReady)
  if ($gotPick -ne $pick -or $gotReady -ne $ready) {
    Write-Output "FAIL $name`: pick=$gotPick ready=$gotReady, want $pick, $ready"
    $script:failed++
  } else { Write-Output "ok   $name" }
}

$current = 'Ubuntu  Ubuntu', 'Ubuntu-26.04  a', 'Ubuntu-24.04  b', 'Ubuntu-22.04  c'
$legacy = 'Ubuntu  Ubuntu', 'Debian  Debian GNU/Linux'
Case 'fresh machine takes the newest offered' $null @('Debian') $current 0 'Ubuntu-26.04' $true
Case 'a later LTS is picked up' $null @() ('Ubuntu-26.04  a', 'Ubuntu-28.04  b') 0 'Ubuntu-28.04' $true
Case 'old WSL lists nothing usable' $null @('Debian') $legacy 0 'Ubuntu-26.04' $false
Case 'a list of only old versions is unusable' $null @() @('Ubuntu-22.04  c') 0 'Ubuntu-26.04' $false
Case 'failed lookup leaves the install to report it' $null @() @() 1 'Ubuntu-26.04' $true
Case 'an installed Ubuntu stays' $null @('Ubuntu-24.04') $current 0 'Ubuntu-24.04' $true
Case 'an old installed Ubuntu does not count' $null @('Ubuntu-22.04') $current 0 'Ubuntu-26.04' $true
Case 'the distro Lerd runs in beats a newer one' 'Ubuntu-24.04' @('Ubuntu-24.04', 'Ubuntu-26.04') $current 0 'Ubuntu-24.04' $true
Case 'the distro Lerd runs in need not be Ubuntu' 'Debian' @('Debian', 'Ubuntu-26.04') $legacy 0 'Debian' $true
Case 'a removed distro in the config is ignored' 'Ubuntu-24.04' @('Ubuntu-26.04') $current 0 'Ubuntu-26.04' $true

Remove-Item $root -Recurse -Force
if ($failed) { exit 1 }
