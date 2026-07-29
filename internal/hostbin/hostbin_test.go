package hostbin

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLookFindsBinaryOnPATH(t *testing.T) {
	dir := t.TempDir()
	writeExe(t, filepath.Join(dir, "lerdtool"))
	t.Setenv("PATH", dir)

	got, ok := Look("lerdtool")
	if !ok || got != filepath.Join(dir, "lerdtool") {
		t.Fatalf("Look() = %q, %v; want the PATH copy", got, ok)
	}
}

// The launchd case: a Homebrew tool with none of its prefixes on PATH. The name
// is synthetic because a machine holding the real tool in a system prefix would
// resolve it there, failing a test that is about the fallback.
func TestLookFallsBackToExtraDirs(t *testing.T) {
	brew := t.TempDir()
	writeExe(t, filepath.Join(brew, "lerdtunnel"))
	withExtraDirs(t, brew)
	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")

	got, ok := Look("lerdtunnel")
	if !ok || got != filepath.Join(brew, "lerdtunnel") {
		t.Fatalf("Look() = %q, %v; want %s", got, ok, filepath.Join(brew, "lerdtunnel"))
	}
}

// A non-executable file of the right name is not a tool.
func TestLookIgnoresNonExecutable(t *testing.T) {
	brew := t.TempDir()
	if err := os.WriteFile(filepath.Join(brew, "cloudflared"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	withExtraDirs(t, brew)
	t.Setenv("PATH", t.TempDir())

	if got, ok := Look("cloudflared"); ok {
		t.Fatalf("Look() = %q, true; want not found", got)
	}
}

func TestLookReportsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got, ok := Look("lerd-no-such-binary-anywhere"); ok {
		t.Fatalf("Look() = %q, true; want not found", got)
	}
}

func TestPathFallsBackToBareName(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got := Path("lerd-no-such-binary-anywhere"); got != "lerd-no-such-binary-anywhere" {
		t.Fatalf("Path() = %q; want the bare name", got)
	}
}

func TestExtraDirsCoverHomebrewOnMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	want := map[string]bool{"/opt/homebrew/bin": false, "/usr/local/bin": false}
	for _, dir := range ExtraDirs() {
		if _, ok := want[dir]; ok {
			want[dir] = true
		}
	}
	for dir, found := range want {
		if !found {
			t.Errorf("ExtraDirs() is missing %s", dir)
		}
	}
}

func withExtraDirs(t *testing.T, dirs ...string) {
	t.Helper()
	prev := ExtraDirs
	ExtraDirs = func() []string { return dirs }
	t.Cleanup(func() { ExtraDirs = prev })
}

func writeExe(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
