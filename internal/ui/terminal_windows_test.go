//go:build windows

package ui

import (
	"encoding/base64"
	"slices"
	"testing"
	"unicode/utf16"
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

func decodePowerShell(t *testing.T, enc string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u))
}

// The dashboard's update button used an sh script, which no Windows terminal
// runs. It now opens PowerShell running lerd update in Windows Terminal, or in a
// console that `start` opens, and waits for Enter before the window closes.
func TestUpdateTerminalsRunLerdUpdateInPowerShell(t *testing.T) {
	got := updateTerminals(`C:\Users\O'Brien\AppData\Local\lerd\bin\lerd.exe`)
	if len(got) != 2 || got[0].bin != "wt.exe" || got[1].bin != "cmd.exe" {
		t.Fatalf("candidates = %+v, want wt.exe then cmd.exe", got)
	}
	if !slices.Equal(got[1].args[:3], []string{"/c", "start", ""}) {
		t.Errorf("cmd.exe args = %q, want start in a new window", got[1].args)
	}
	for _, c := range got {
		enc := c.args[len(c.args)-1]
		if c.args[len(c.args)-2] != "-EncodedCommand" {
			t.Fatalf("%s args = %q, want an encoded command last", c.bin, c.args)
		}
		want := `& 'C:\Users\O''Brien\AppData\Local\lerd\bin\lerd.exe' update; Write-Host; Read-Host 'Press Enter to close'`
		if script := decodePowerShell(t, enc); script != want {
			t.Errorf("%s script = %q, want %q", c.bin, script, want)
		}
	}
}

// Terminal-mode commands open PowerShell at the project directory and hold the
// window until Enter, in place of the sh script no Windows terminal runs.
func TestCommandTerminalsRunTheCommandInPowerShell(t *testing.T) {
	got := commandTerminals(`C:\Sites\o'brien`, "php artisan native:jump")
	if len(got) != 2 || got[0].bin != "wt.exe" || got[1].bin != "cmd.exe" {
		t.Fatalf("candidates = %+v, want wt.exe then cmd.exe", got)
	}
	want := "Set-Location -LiteralPath 'C:\\Sites\\o''brien'\nphp artisan native:jump\nWrite-Host; Read-Host '[press Enter to close]'"
	for _, c := range got {
		if script := decodePowerShell(t, c.args[len(c.args)-1]); script != want {
			t.Errorf("%s script = %q, want %q", c.bin, script, want)
		}
	}
}
