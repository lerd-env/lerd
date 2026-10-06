//go:build linux

package unitlog

// IsContainerUnit returns true on Linux for every unit but lerd-dns, the one
// lerd unit that runs as a host process and logs only to the journal.
func IsContainerUnit(unit string) bool { return unit != "lerd-dns" }

// LogHint is the command a user runs to read a unit's recent output. It lives
// here rather than beside each caller so the darwin build cannot end up naming
// journalctl, which no Mac has.
func LogHint(unit string) string {
	return "journalctl --user -u " + unit + " -n 20"
}
