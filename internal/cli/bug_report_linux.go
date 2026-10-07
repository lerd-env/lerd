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
