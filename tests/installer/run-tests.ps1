# Runs the install.ps1 tests with Pester 5, installing it for the current user
# when missing, and exits non-zero when a test fails.
# Run: powershell -File tests/installer/run-tests.ps1   (or pwsh -File ...)

$ErrorActionPreference = 'Stop'
if (-not (Get-Module -ListAvailable Pester | Where-Object { $_.Version -ge [version]'5.5' })) {
  Install-Module Pester -MinimumVersion 5.5 -Scope CurrentUser -Force -SkipPublisherCheck
}
Import-Module Pester -MinimumVersion 5.5

$config = New-PesterConfiguration
$config.Run.Path = Join-Path $PSScriptRoot 'install.Tests.ps1'
$config.Run.Exit = $true
$config.Output.Verbosity = 'Detailed'
Invoke-Pester -Configuration $config
