//go:build !windows

package ui

// platformTerminals has nothing to add here: Linux and macOS terminals come
// from terminalDirCandidates itself.
func platformTerminals(string) []terminalCmd { return nil }

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
