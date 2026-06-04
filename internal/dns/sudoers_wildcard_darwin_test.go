//go:build darwin

package dns

import (
	"strings"
	"testing"
)

func TestRenderDarwinSudoers_NoWildcardArgs(t *testing.T) {
	content := renderDarwinSudoers("alice", []string{"test"})
	assertNoWildcardArgs(t, content)

	// Still wildcard-free with several TLDs.
	assertNoWildcardArgs(t, renderDarwinSudoers("alice", []string{"test", "lan", "lab"}))
}

func TestRenderDarwinSudoers_UsesConfiguredTLDPath(t *testing.T) {
	content := renderDarwinSudoers("alice", []string{"lan"})
	if !strings.Contains(content, "/etc/resolver/lan") {
		t.Errorf("rendered content should reference /etc/resolver/lan, got: %s", content)
	}
	if strings.Contains(content, "/etc/resolver/test") {
		t.Errorf("rendered content should not reference /etc/resolver/test when tld=lan, got: %s", content)
	}
}

func TestRenderDarwinSudoers_PerTLDRules(t *testing.T) {
	content := renderDarwinSudoers("alice", []string{"test", "local"})
	for _, want := range []string{
		"/usr/bin/tee /etc/resolver/test",
		"/bin/chmod 644 /etc/resolver/test",
		"/usr/bin/tee /etc/resolver/local",
		"/bin/chmod 644 /etc/resolver/local",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("rendered sudoers missing %q in:\n%s", want, content)
		}
	}
}
