//go:build linux

package unitlog

// IsContainerUnit returns true on Linux — all lerd units run as Podman
// containers, so their logs come from `podman logs` / the journal.
func IsContainerUnit(_ string) bool { return true }

// LogHint is the command a user runs to read a unit's recent output. It lives
// here rather than beside each caller so the darwin build cannot end up naming
// journalctl, which no Mac has.
func LogHint(unit string) string {
	return "journalctl --user -u " + unit + " -n 20"
}
