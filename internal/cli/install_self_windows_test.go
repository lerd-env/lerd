//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlaceBinaryCopiesLerdAndTheTrayIntoBin(t *testing.T) {
	src, bin := t.TempDir(), t.TempDir()
	for name, body := range map[string]string{"lerd.exe": "new lerd", "lerd-tray.exe": "new tray"} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "lerd.exe"), []byte("old lerd"), 0o755); err != nil {
		t.Fatal(err)
	}
	dest, err := placeBinary(filepath.Join(src, "lerd.exe"), bin)
	if err != nil {
		t.Fatal(err)
	}
	if dest != filepath.Join(bin, "lerd.exe") {
		t.Errorf("dest = %s", dest)
	}
	for name, want := range map[string]string{"lerd.exe": "new lerd", "lerd-tray.exe": "new tray"} {
		if got, _ := os.ReadFile(filepath.Join(bin, name)); string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestPlaceBinaryWorksWithoutATrayBeside(t *testing.T) {
	src, bin := t.TempDir(), filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(filepath.Join(src, "lerd.exe"), []byte("lerd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := placeBinary(filepath.Join(src, "lerd.exe"), bin); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bin, "lerd-tray.exe")); !os.IsNotExist(err) {
		t.Errorf("a tray appeared from nowhere: %v", err)
	}
}

// A running lerd.exe cannot be overwritten on Windows, only renamed, so the
// one in bin is moved aside first and the copies a previous run left behind
// are cleared once nothing holds them.
func TestPlaceBinaryMovesTheOldCopyAsideAndClearsStaleOnes(t *testing.T) {
	src, bin := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "lerd.exe"), []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(bin, "lerd.exe.old-1")
	for _, p := range []string{filepath.Join(bin, "lerd.exe"), stale} {
		if err := os.WriteFile(p, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := placeBinary(filepath.Join(src, "lerd.exe"), bin); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale copy %s was kept", stale)
	}
	if got, _ := os.ReadFile(filepath.Join(bin, "lerd.exe")); string(got) != "new" {
		t.Errorf("lerd.exe = %q", got)
	}
}

func TestSameExecutableIgnoresCaseAndSeparators(t *testing.T) {
	if !sameExecutable(`C:\Users\Me\AppData\Local\lerd\bin\lerd.exe`, `c:/users/me/appdata/local/lerd/bin/LERD.EXE`) {
		t.Error("the same file spelled differently was treated as another")
	}
	if sameExecutable(`C:\Users\me\Downloads\lerd.exe`, `C:\Users\me\AppData\Local\lerd\bin\lerd.exe`) {
		t.Error("a download was treated as the installed copy")
	}
}
