//go:build windows

package ui

import (
	"slices"
	"testing"
)

// None of the Linux or macOS emulators exist on Windows, so the site's terminal
// button found nothing. Windows Terminal is tried first, then a PowerShell
// console that `start` opens in its own window at the site's directory.
func TestPlatformTerminalsOnWindows(t *testing.T) {
	dir := `C:\Users\me\Sites\app`
	got := platformTerminals(dir)
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(got), got)
	}
	if got[0].bin != "wt.exe" || !slices.Equal(got[0].args, []string{"-d", dir}) {
		t.Errorf("first candidate = %+v, want Windows Terminal at the dir", got[0])
	}
	want := []string{"/c", "start", "", "/D", dir, "powershell", "-NoLogo"}
	if got[1].bin != "cmd.exe" || !slices.Equal(got[1].args, want) {
		t.Errorf("fallback = %+v, want cmd.exe %q", got[1], want)
	}
}

func TestTerminalDirCandidatesIncludeTheWindowsTerminals(t *testing.T) {
	t.Setenv("TERMINAL", "")
	got := terminalDirCandidates(`C:\Sites\app`)
	if !slices.ContainsFunc(got, func(c terminalCmd) bool { return c.bin == "wt.exe" }) {
		t.Errorf("candidates %+v do not offer Windows Terminal", got)
	}
}
