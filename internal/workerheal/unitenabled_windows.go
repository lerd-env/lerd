//go:build windows

package workerheal

// isUnitEnabled is unknown on Windows, as on macOS: stopped workers are never
// flagged as drift.
func isUnitEnabled(string) bool { return false }
