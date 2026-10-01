//go:build windows

package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/services"
)

// The exe path goes into the unit as ExecStart and comes back out through
// services.UnitExecBinary. Escaping backslashes on the way in (as %q does) would
// hand Windows a path with doubled separators.
func TestDNSServiceContentRoundTripsAWindowsExePath(t *testing.T) {
	for _, exe := range []string{
		`C:\Users\me\AppData\Local\lerd\bin\lerd.exe`,
		`C:\Users\me\My Tools\lerd.exe`,
	} {
		content := dnsServiceContent(exe)
		if got := services.UnitExecBinary(content); got != exe {
			t.Errorf("ExecStart path = %q, want %q\n%s", got, exe, content)
		}
		args := services.SplitExecStart(execStartLine(content))
		if len(args) != 2 || args[1] != "dns-serve" {
			t.Errorf("args = %q, want [exe dns-serve]", args)
		}
	}
}

func execStartLine(content string) string {
	const key = "ExecStart="
	for _, l := range splitLines(content) {
		if len(l) > len(key) && l[:len(key)] == key {
			return l[len(key):]
		}
	}
	return ""
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
