//go:build windows

package node

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mise installs as mise.exe on Windows, and Windows files carry no execute
// bits, so a probe for ~/.local/bin/mise with mode&0o111 never found it.
func TestMiseInstallPathOnWindows(t *testing.T) {
	if p := miseInstallPath(`C:\Users\me`); !strings.HasSuffix(p, `\.local\bin\mise.exe`) {
		t.Errorf("miseInstallPath = %q, want ~/.local/bin/mise.exe", p)
	}
}

func TestFindMiseFindsTheWindowsInstall(t *testing.T) {
	home := t.TempDir()
	exe := miseInstallPath(home)
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	notOnPath := func(string) (string, error) { return "", os.ErrNotExist }
	if got := findMise(home, notOnPath); got != exe {
		t.Errorf("findMise = %q, want %q", got, exe)
	}
	if isExecutableFile(filepath.Dir(exe)) {
		t.Error("a directory is not an executable")
	}
}
