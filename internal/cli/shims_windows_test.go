//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PowerShell and cmd only run a shim with an extension from PATHEXT, so each sh
// shim needs a .cmd twin that hands its arguments to the same lerd subcommand.
func TestCmdShimRunsTheLerdSubcommand(t *testing.T) {
	got := cmdShim(`C:\lerd\lerd.exe`, `C:\fallback\lerd.exe`, "php")
	for _, want := range []string{
		"@echo off\r\n",
		`set "LERD=C:\lerd\lerd.exe"`,
		`if not exist "%LERD%" set "LERD=C:\fallback\lerd.exe"`,
		`"%LERD%" php %*`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("cmdShim missing %q in:\n%s", want, got)
		}
	}
}

func TestWriteCmdShimsWritesAndRemoves(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "node.cmd")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeCmdShims(dir, `C:\lerd\lerd.exe`, map[string]string{"php": "php", "node": ""})
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "php.cmd")); err != nil || !strings.Contains(string(data), `"%LERD%" php %*`) {
		t.Errorf("php.cmd = %q, %v", data, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("node.cmd should be removed when its command is empty, stat err = %v", err)
	}
}

func TestPathListEntries(t *testing.T) {
	dir := `C:\Users\me\AppData\Local\lerd\bin`
	cases := []struct{ name, in, add, remove string }{
		{"empty", "", dir, ""},
		{"prepends", `C:\a;C:\b`, dir + `;C:\a;C:\b`, `C:\a;C:\b`},
		{"already there, any case or trailing slash", `C:\a;c:\users\me\appdata\local\lerd\bin\;C:\b`, `C:\a;c:\users\me\appdata\local\lerd\bin\;C:\b`, `C:\a;C:\b`},
		{"drops empty items", `C:\a;;C:\b;`, dir + `;C:\a;C:\b`, `C:\a;C:\b`},
	}
	for _, c := range cases {
		if got := withPathEntry(c.in, dir); got != c.add {
			t.Errorf("%s: withPathEntry = %q, want %q", c.name, got, c.add)
		}
		if got := withoutPathEntry(c.in, dir); got != c.remove {
			t.Errorf("%s: withoutPathEntry = %q, want %q", c.name, got, c.remove)
		}
	}
}
