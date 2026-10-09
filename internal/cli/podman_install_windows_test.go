//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindPodmanPutsAnInstalledPodmanOnPath(t *testing.T) {
	empty, dir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "podman.exe"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", empty)
	if !findPodman([]string{filepath.Join(empty, "missing"), dir}) {
		t.Fatal("a podman.exe in an install folder was not found")
	}
	if !strings.HasPrefix(os.Getenv("PATH"), dir+string(os.PathListSeparator)) {
		t.Errorf("PATH = %q, want it to start with %s", os.Getenv("PATH"), dir)
	}
}

func TestFindPodmanReportsAMissingInstall(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	if findPodman([]string{filepath.Join(empty, "Podman")}) {
		t.Error("found a podman that is not there")
	}
	if os.Getenv("PATH") != empty {
		t.Errorf("PATH changed to %q", os.Getenv("PATH"))
	}
}

func TestPodmanInstallDirsCoverTheMSIScopes(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\me\AppData\Local`)
	t.Setenv("ProgramFiles", `C:\Program Files`)
	dirs := podmanInstallDirs()
	for _, want := range []string{`C:\Users\me\AppData\Local\Programs\Podman`, `C:\Program Files\RedHat\Podman`} {
		found := false
		for _, d := range dirs {
			found = found || d == want
		}
		if !found {
			t.Errorf("install dirs %v miss %s", dirs, want)
		}
	}
}

func TestMSIExitCodes(t *testing.T) {
	for code, ok := range map[int]bool{0: true, 3010: true, 1602: false, 1618: false, 1603: false} {
		err := msiResult(code, `C:\logs\podman-install.log`)
		if (err == nil) != ok {
			t.Errorf("exit %d: err = %v, want ok=%v", code, err, ok)
		}
		if err != nil && !strings.Contains(err.Error(), "podman-install.log") {
			t.Errorf("exit %d: %q does not point at the install log", code, err)
		}
	}
	if err := msiResult(1618, "x"); !strings.Contains(err.Error(), "another installation") {
		t.Errorf("1618 should say another install is running: %v", err)
	}
}
