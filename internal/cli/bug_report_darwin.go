package cli

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// writeHostDetails adds the macOS version to the bug report header.
func writeHostDetails(w io.Writer) {
	if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
		fmt.Fprintf(w, "macOS:      %s\n", strings.TrimSpace(string(out)))
	}
}

// writePlatformSections has nothing to add on macOS.
func writePlatformSections(io.Writer) {}

func dumpHostLogs(w io.Writer, _ int, _ *logFilter) {
	fmt.Fprintln(w, "(skipped: journalctl is Linux-only)")
}
