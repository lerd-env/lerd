//go:build !windows

package ui

// platformTerminals has nothing to add here: Linux and macOS terminals come
// from terminalDirCandidates itself.
func platformTerminals(string) []terminalCmd { return nil }
