package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFlatpak puts a flatpak on PATH that logs each call's arguments, one line
// per call, and returns the log's path.
func fakeFlatpak(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "flatpak"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return log
}

func flatpakCalls(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestRemoveDesktopFlatpak_ClosesThenRemovesUserInstall(t *testing.T) {
	log := fakeFlatpak(t)
	if err := removeDesktopFlatpak(false); err != nil {
		t.Fatal(err)
	}
	calls := flatpakCalls(t, log)
	want := []string{
		"kill sh.lerd.Desktop",
		"uninstall --user -y --noninteractive sh.lerd.Desktop",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls = %q, want %q", calls, want)
	}
}

// The app's own settings go only when the person chose to remove Lerd's data.
func TestRemoveDesktopFlatpak_DeletesAppDataOnlyWithLerdData(t *testing.T) {
	log := fakeFlatpak(t)
	if err := removeDesktopFlatpak(true); err != nil {
		t.Fatal(err)
	}
	calls := flatpakCalls(t, log)
	if last := calls[len(calls)-1]; !strings.Contains(last, "--delete-data") {
		t.Errorf("uninstall = %q, want --delete-data", last)
	}
}
