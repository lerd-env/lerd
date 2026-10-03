//go:build windows

package ui

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf16"
)

// platformTerminals offers Windows Terminal at dir, then a PowerShell console
// that `start` opens in its own window there. lerd-ui runs with no console of
// its own, so a console program has to be given a new window explicitly.
func platformTerminals(dir string) []terminalCmd {
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
	script := "& '" + strings.ReplaceAll(self, "'", "''") + "' update; Write-Host; Read-Host 'Press Enter to close'"
	ps := []string{"powershell", "-NoLogo", "-NoProfile", "-EncodedCommand", encodePowerShell(script)}
	return []terminalCmd{
		{"wt.exe", ps},
		{"cmd.exe", append([]string{"/c", "start", ""}, ps...)},
	}
}

// encodePowerShell is the base64 of the UTF-16LE text -EncodedCommand expects.
func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}
