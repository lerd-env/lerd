//go:build !windows

package cli

// The sh shims and the shell rc PATH entry already cover these hosts.
func writeCmdShims(string, string, map[string]string) error { return nil }

func writeUserPathEntry(string) (bool, error) { return false, nil }

func removeUserPathEntry(string) bool { return false }

// installSelf has nothing to do here: install.sh and Homebrew place the binary.
func installSelf() (bool, error) { return false, nil }
