//go:build !windows

package ui

// openUpdateTerminal runs `lerd update` in a new terminal window.
func openUpdateTerminal(self string) error {
	return openTerminalCommand(buildUpdateScript(self))
}

// openLogTerminal follows a unit's logs in a new terminal window.
func openLogTerminal(script string) error { return openTerminalCommand(script) }

// openCommandTerminal runs a site command in a new terminal window at cwd.
func openCommandTerminal(cwd, command string) error {
	return openTerminalCommand(terminalCommandScript(cwd, command))
}
