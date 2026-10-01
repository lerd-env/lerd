//go:build windows

package ui

// platformTerminals offers Windows Terminal at dir, then a PowerShell console
// that `start` opens in its own window there. lerd-ui runs with no console of
// its own, so a console program has to be given a new window explicitly.
func platformTerminals(dir string) []terminalCmd {
	return []terminalCmd{
		{"wt.exe", []string{"-d", dir}},
		{"cmd.exe", []string{"/c", "start", "", "/D", dir, "powershell", "-NoLogo"}},
	}
}
