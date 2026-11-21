package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeFlatpak puts a flatpak on PATH with the given user refs installed. It logs
// each call's arguments, one line per call, and returns the log's path. Like the
// real one, info refuses a bare app id that has more than one branch.
func fakeFlatpak(t *testing.T, refs ...string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	branches := 0
	for _, r := range refs {
		if strings.HasPrefix(r, desktopFlatpakID+"/") {
			branches++
		}
	}
	script := "#!/bin/sh\necho \"$*\" >> \"${0%/*}/calls.log\"\n" +
		"case \"$1\" in\n" +
		"list) printf '%s\\n' " + strings.Join(refs, " ") + " ;;\n" +
		"info) [ " + strconv.Itoa(branches) + " -eq 1 ] || exit 1 ;;\n" +
		"esac\n"
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

const (
	desktopStable = "sh.lerd.Desktop/x86_64/stable"
	desktopMaster = "sh.lerd.Desktop/x86_64/master"
)

func TestDesktopFlatpakInstalled_OneBranch(t *testing.T) {
	fakeFlatpak(t, "org.example.Other/x86_64/stable", desktopStable)
	if !desktopFlatpakInstalled() {
		t.Error("an installed desktop app was not found")
	}
}

// flatpak info refuses an app id with two branches, which read as not installed.
func TestDesktopFlatpakInstalled_TwoBranches(t *testing.T) {
	fakeFlatpak(t, desktopMaster, desktopStable)
	if !desktopFlatpakInstalled() {
		t.Error("a desktop app installed on two branches was not found")
	}
}

func TestDesktopFlatpakInstalled_OnlyOtherApps(t *testing.T) {
	fakeFlatpak(t, "org.example.Other/x86_64/stable", "sh.lerd.DesktopBeta/x86_64/stable")
	if desktopFlatpakInstalled() {
		t.Error("another app was taken for the desktop app")
	}
}

func TestRemoveDesktopFlatpak_ClosesThenRemovesEveryBranch(t *testing.T) {
	log := fakeFlatpak(t, desktopMaster, desktopStable)
	if err := removeDesktopFlatpak(false); err != nil {
		t.Fatal(err)
	}
	calls := flatpakCalls(t, log)
	want := []string{
		"list --user --app --columns=ref",
		"kill sh.lerd.Desktop",
		"uninstall --user -y --noninteractive " + desktopMaster + " " + desktopStable,
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls = %q, want %q", calls, want)
	}
}

// The app's own settings go only when the person chose to remove Lerd's data.
func TestRemoveDesktopFlatpak_DeletesAppDataOnlyWithLerdData(t *testing.T) {
	log := fakeFlatpak(t, desktopStable)
	if err := removeDesktopFlatpak(true); err != nil {
		t.Fatal(err)
	}
	calls := flatpakCalls(t, log)
	if last := calls[len(calls)-1]; !strings.Contains(last, "--delete-data") {
		t.Errorf("uninstall = %q, want --delete-data", last)
	}
}
