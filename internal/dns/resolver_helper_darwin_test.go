//go:build darwin

package dns

import (
	"strings"
	"testing"
)

func TestResolverHelperScript_safeguards(t *testing.T) {
	s := resolverHelperScript()

	for _, want := range []string{
		"#!/bin/sh",
		"set -eu",
		"nameserver 127.0.0.1", // fixed content, caller can't inject the body
		"port 5300",
		"/etc/resolver/",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("helper script missing %q", want)
		}
	}

	// Deny-list must cover real public suffixes and the OS-owned specials so a
	// background process can't reroute real-internet DNS or the mDNS/loopback
	// special-cases.
	for _, tld := range []string{"com", "dev", "io", "co", "local", "localhost"} {
		if !strings.Contains(s, " "+tld+" ") {
			t.Errorf("helper deny-list missing %q", tld)
		}
	}

	// Single-label validation: must reject anything outside [a-z0-9-] (which
	// rules out dots / path separators), so it can only touch /etc/resolver/<label>.
	if !strings.Contains(s, "*[!a-z0-9-]*") {
		t.Errorf("helper script must validate the label charset")
	}

	// Prune must compare against lerd's exact content, never blanket-delete.
	if !strings.Contains(s, `"$(cat "$f" 2>/dev/null)" = "$CONTENT"`) {
		t.Errorf("helper prune must match lerd's exact resolver content")
	}
}

func TestRenderDarwinSudoers_includesHelperGrant(t *testing.T) {
	content := renderDarwinSudoers("alice", []string{"test"})
	want := "alice ALL=(root) NOPASSWD: " + resolverHelperPath
	if !strings.Contains(content, want) {
		t.Errorf("sudoers missing helper grant %q in:\n%s", want, content)
	}
	// The grant must remain wildcard-free (it passes strict sudo parsers).
	assertNoWildcardArgs(t, content)
}
