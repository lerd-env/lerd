# Tests what install.ps1 says when Windows refuses to elevate. The functions are
# lifted out of the script, which runs on load, and Start-Process is stubbed to
# fail the way Windows does.
$ErrorActionPreference = 'Stop'
$script = Get-Content (Join-Path $PSScriptRoot '..\..\install.ps1') -Raw
$start = $script.IndexOf('  # Refusal words')
if ($start -lt 0) { $start = $script.IndexOf('  # Elevated runs a PowerShell script') }
$end = $script.IndexOf('  function IsAdmin')
Invoke-Expression $script.Substring($start, $end - $start)
function Lit([string]$s) { "'" + ($s -replace "'", "''") + "'" }

$failed = 0
function Check($name, $got, $parts, $hint) {
  $missing = @($parts | Where-Object { -not $got.Contains($_) })
  if ($missing -or ($got.Contains('Windows PowerShell 5.1') -ne $hint)) {
    Write-Output "FAIL $name`: $got"
    $script:failed++
  } else { Write-Output "ok   $name" }
}

$base = 'Windows did not grant administrator rights'
$licence = 'No applicable app licenses found.'
function Start-Process { throw $licence }
try { $null = Elevated 'exit 0'; $got = '' } catch { $got = $_.Exception.Message }
Check 'elevation shows what Windows said' $got @($base, $licence) ($PSVersionTable.PSEdition -eq 'Core')
Check '5.1 gets no hint' (Refusal $licence 'Desktop') @($base, $licence) $false
Check 'PowerShell 7 is pointed at 5.1' (Refusal $licence 'Core') @($base, $licence) $true
Check 'a declined prompt on PowerShell 7 too' (Refusal 'The operation was canceled by the user.' 'Core') @($base, 'canceled by the user') $true
exit $failed
