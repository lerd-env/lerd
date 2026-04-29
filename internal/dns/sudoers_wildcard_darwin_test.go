//go:build darwin

package dns

import (
	"strings"
	"testing"
)

func TestRenderDarwinSudoers_NoWildcardArgs(t *testing.T) {
	content := renderDarwinSudoers("alice", "test")
	assertNoWildcardArgs(t, content)
}

func TestRenderDarwinSudoers_UsesConfiguredTLDPath(t *testing.T) {
	content := renderDarwinSudoers("alice", "lan")
	if !strings.Contains(content, "/etc/resolver/lan") {
		t.Errorf("rendered content should reference /etc/resolver/lan, got: %s", content)
	}
	if strings.Contains(content, "/etc/resolver/test") {
		t.Errorf("rendered content should not reference /etc/resolver/test when tld=lan, got: %s", content)
	}
}
