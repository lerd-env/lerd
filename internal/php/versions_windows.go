//go:build windows

package php

// listInstalledFromServiceDir is a no-op on Windows; PHP versions are found
// through the container list, as on Linux.
func listInstalledFromServiceDir() []string { return nil }
