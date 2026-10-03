//go:build !windows

package cli

// removeAfterExit is never needed here: a running binary can be unlinked.
func removeAfterExit(string) bool { return false }
