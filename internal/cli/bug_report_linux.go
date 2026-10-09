package cli

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// writeHostDetails adds the distro and kernel to the bug report header.
func writeHostDetails(w io.Writer) {
	if name := readOSRelease(); name != "" {
		fmt.Fprintf(w, "Distro:     %s\n", name)
	}
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		fmt.Fprintf(w, "Kernel:     %s\n", strings.TrimSpace(string(out)))
	}
}

// writePlatformSections has nothing to add on Linux.
func writePlatformSections(io.Writer) {}

// dumpHostLogs reads each lerd infra unit's recent journal.
func dumpHostLogs(w io.Writer, n int, filter *logFilter) {
	for _, unit := range lerdUnits() {
		if isContentUnit(unit) {
			continue
		}
		fmt.Fprintf(w, "── journalctl --user -u %s --no-pager -n %d\n", unit, n)
		out, err := exec.Command("journalctl", "--user", "-u", unit,
			"--no-pager", "-n", fmt.Sprintf("%d", n)).CombinedOutput()
		if err != nil {
			fmt.Fprintf(w, "(failed: %v)\n", err)
		}
		cleaned := filter.clean(strings.TrimRight(string(out), "\n"))
		fmt.Fprintln(w, cleaned)
		fmt.Fprintln(w)
	}
}
