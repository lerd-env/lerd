# Submitting a Pull Request

## Before opening a PR

### Formatting

All Go code must be formatted with `gofmt`. Check and fix before committing:

```bash
gofmt -l .   # list files with issues
gofmt -w .   # fix them
```

The CI will reject PRs with unformatted code.

### Tests

Run the full test suite:

```bash
make test
```

**New functionality must include tests.** PRs that add or change behaviour without corresponding test coverage will not be merged.

### Vet

```bash
CGO_ENABLED=1 go vet ./...
```

## CI checks

Every PR runs the following checks automatically. All must pass before merging.

| Check | Command |
|-------|---------|
| Build | `go build ./cmd/lerd` |
| Tests | `go test ./...` |
| Vet | `go vet ./...` |
| Format | `gofmt -l .` |
| Installer tests | `bats tests/installer/installer.bats` |
| Windows build and vet | `go build` and `go vet` for `GOOS=windows`, amd64 and arm64 |
| Windows tests | `.github/scripts/windows-tests.sh`, every test in a `*_windows_test.go` file |
| Windows installer tests | `tests/installer/run-tests.ps1` under Windows PowerShell 5.1 and PowerShell 7 |
| Windows installer lint | `Invoke-ScriptAnalyzer -Path install.ps1 -Settings tests/installer/PSScriptAnalyzerSettings.psd1` |
