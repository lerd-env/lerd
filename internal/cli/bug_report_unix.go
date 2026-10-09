//go:build !windows

package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// dumpResolverConfig shows the host resolver lerd's DNS sits behind.
func dumpResolverConfig(w io.Writer) {
	fmt.Fprintln(w, "── /etc/resolv.conf")
	if data, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		fmt.Fprintln(w, redactResolvConf(strings.TrimRight(string(data), "\n")))
	} else {
		fmt.Fprintf(w, "(unreadable: %v)\n", err)
	}
}
