//go:build windows

package ui

import (
	"fmt"

	"github.com/geodro/lerd/internal/hostshell"
)

// osDefaultDirTerminal has no desktop-wide terminal setting to read on Windows.
func osDefaultDirTerminal(string) []terminalCmd { return nil }

// osFallbackDirTerminals offers Windows Terminal at dir, then a PowerShell
// console that `start` opens in its own window there. lerd-ui runs with no
// console of its own, so a console program has to be given a new window.
func osFallbackDirTerminals(dir string) []terminalCmd {
	return []terminalCmd{
		{"wt.exe", []string{"-d", dir}},
		{"cmd.exe", []string{"/c", "start", "", "/D", dir, "powershell", "-NoLogo"}},
	}
}

// openUpdateTerminal runs `lerd update` in Windows Terminal, or a PowerShell
// console of its own, and keeps the window open until Enter is pressed.
func openUpdateTerminal(self string) error {
	started, err := startFirstTerminal(updateTerminals(self))
	if !started && err == nil {
		return fmt.Errorf("no terminal found; run lerd update from a terminal")
	}
	return err
}

// updateTerminals passes the script encoded, so neither wt.exe, which splits
// its command line on ';', nor cmd.exe gets to reinterpret it.
func updateTerminals(self string) []terminalCmd {
	script := "& " + hostshell.Quote(self) + " update; Write-Host; Read-Host 'Press Enter to close'"
	ps := []string{"powershell", "-NoLogo", "-NoProfile", "-EncodedCommand", hostshell.Encode(script)}
	return []terminalCmd{
		{"wt.exe", ps},
		{"cmd.exe", append([]string{"/c", "start", ""}, ps...)},
	}
}

// openCommandTerminal runs a site command in PowerShell at cwd, in Windows
// Terminal or a console of its own, and keeps the window open until Enter is
// pressed. No Windows terminal runs the sh script the other platforms use.
func openCommandTerminal(cwd, command string) error {
	started, err := startFirstTerminal(commandTerminals(cwd, command))
	if !started && err == nil {
		return fmt.Errorf("no terminal found; run %s from a terminal", command)
	}
	return err
}

func commandTerminals(cwd, command string) []terminalCmd {
	return powerShellTerminals("Set-Location -LiteralPath " + hostshell.Quote(cwd) + "\n" + command +
		"\nWrite-Host; Read-Host '[press Enter to close]'")
}

// openLogTerminal follows a unit's logs in Windows Terminal, or a PowerShell
// console of its own. The follow scripts are PowerShell already; what the
// button lacked was a window, since only Linux emulators were tried.
func openLogTerminal(script string) error {
	started, err := startFirstTerminal(powerShellTerminals(script))
	if !started && err == nil {
		return fmt.Errorf("no terminal found; follow the logs from a terminal")
	}
	return err
}

// powerShellTerminals runs script in Windows Terminal, then in a console that
// `start` opens, passing it encoded for the same reason updateTerminals does.
func powerShellTerminals(script string) []terminalCmd {
	ps := []string{hostshell.Bin(), "-NoLogo", "-NoProfile", "-EncodedCommand", hostshell.Encode(script)}
	return []terminalCmd{
		{"wt.exe", ps},
		{"cmd.exe", append([]string{"/c", "start", ""}, ps...)},
	}
}

// defaultTerminal is empty on Windows, which has no desktop terminal setting.
var defaultTerminal = func() string { return "" }

func osFallbackScriptTerminals(string) []terminalCmd { return nil }

// terminalBaseEnv carries lerd's environment into a spawned terminal.
func terminalBaseEnv() []string { return graphicalEnv() }
