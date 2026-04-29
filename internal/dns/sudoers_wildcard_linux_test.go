//go:build linux

package dns

import (
	"strings"
	"testing"
)

func TestRenderLinuxSudoers_NoWildcardArgs(t *testing.T) {
	content := renderLinuxSudoers("alice")
	assertNoWildcardArgs(t, content)
}

func TestRenderLinuxSudoers_IncludesUserOnEveryRule(t *testing.T) {
	content := renderLinuxSudoers("alice")
	rules := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "NOPASSWD:") {
			rules++
			if !strings.HasPrefix(line, "alice ") {
				t.Errorf("rule does not start with the user: %q", line)
			}
		}
	}
	if rules == 0 {
		t.Fatal("expected at least one sudoers rule, got none")
	}
}
