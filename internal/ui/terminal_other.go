//go:build !windows

package ui

// platformTerminals has nothing to add here: Linux and macOS terminals come
// from terminalDirCandidates itself.
func platformTerminals(string) []terminalCmd { return nil }

// openUpdateTerminal runs `lerd update` in a new terminal window.
func openUpdateTerminal(self string) error {
	return openTerminalCommand(buildUpdateScript(self))
}

// openCommandTerminal runs a site command in a new terminal window at cwd.
func openCommandTerminal(cwd, command string) error {
	return openTerminalCommand(terminalCommandScript(cwd, command))
}
