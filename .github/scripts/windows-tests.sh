#!/usr/bin/env bash
set -euo pipefail

# Runs every test declared in a *_windows_test.go file, package by package. The
# shared suite still assumes Linux in places, so only the tests written for
# Windows have to pass here; a new one is picked up without listing it.

status=0
while IFS= read -r dir; do
  names=$(grep -ho '^func Test[A-Za-z0-9_]*' "$dir"/*_windows_test.go | sed 's/^func //' | sort -u | paste -sd'|' -)
  [ -n "$names" ] || continue
  echo "::group::./$dir"
  if ! go test -count=1 -run "^($names)\$" "./$dir"; then
    status=1
  fi
  echo "::endgroup::"
done < <(find internal cmd -name '*_windows_test.go' -exec dirname {} \; | sort -u)
exit $status
