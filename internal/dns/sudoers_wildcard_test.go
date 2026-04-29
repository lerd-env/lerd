package dns

import (
	"strings"
	"testing"
)

// assertNoWildcardArgs walks the sudoers drop-in line by line and fails the
// test if any command argument (token after the verb) contains a sudo
// wildcard character. Modern strict parsers — sudo-rs (Ubuntu 26.04 LTS+)
// and C sudo from 1.9.16 onward (Fedora 41+, Arch / CachyOS, openSUSE
// Tumbleweed, NixOS unstable) — hard-reject wildcards in command
// arguments and fall back to the password-prompt path on every call,
// which is what bug #269 reported. Catching this in CI prevents a
// regression that would silently break installs on those distros.
//
// Shared helper used from the platform-tagged test files
// (sudoers_wildcard_linux_test.go, sudoers_wildcard_darwin_test.go) so
// the file stays buildable on both platforms while the renderers it
// exercises are platform-specific.
func assertNoWildcardArgs(t *testing.T, content string) {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Sudoers rule: <user> ALL=(root) NOPASSWD: /path/to/cmd args...
		idx := strings.Index(line, "NOPASSWD:")
		if idx < 0 {
			continue
		}
		cmd := strings.TrimSpace(line[idx+len("NOPASSWD:"):])
		// Multiple commands can appear on one line separated by ", ".
		for _, c := range strings.Split(cmd, ", ") {
			tokens := strings.Fields(c)
			if len(tokens) <= 1 {
				continue
			}
			for _, arg := range tokens[1:] {
				if strings.ContainsAny(arg, "*?") {
					t.Errorf("wildcard in sudoers command argument: %q (full line: %q)", arg, line)
				}
			}
		}
	}
}
