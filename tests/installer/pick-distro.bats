#!/usr/bin/env bats
# Runs install.ps1's distro picker tests under PowerShell.

@test "install.ps1 picks the right distro" {
  command -v pwsh >/dev/null || skip "pwsh not installed"
  run pwsh -NoProfile -File "$BATS_TEST_DIRNAME/pick-distro.ps1"
  echo "$output"
  [ "$status" -eq 0 ]
}
