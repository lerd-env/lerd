#!/usr/bin/env bats
# Runs install.ps1's elevation tests under PowerShell.

@test "install.ps1 says why Windows refused to elevate" {
  command -v pwsh >/dev/null || skip "pwsh not installed"
  run pwsh -NoProfile -File "$BATS_TEST_DIRNAME/elevation.ps1"
  echo "$output"
  [ "$status" -eq 0 ]
}
