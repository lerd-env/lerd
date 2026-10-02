//go:build !windows

package cli

// The sh shims and the shell rc PATH entry already cover these hosts.
func writeCmdShims(string, string, map[string]string) error { return nil }

func writeUserPathEntry(string) (bool, error) { return false, nil }

func removeUserPathEntry(string) bool { return false }
