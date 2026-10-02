//go:build windows

package dns

import "testing"

func TestPortIs53OnWindows(t *testing.T) {
	if Port() != 53 {
		t.Errorf("Port() = %d, want 53: NRPT cannot name any other port", Port())
	}
}

// Windows routes .test through an NRPT rule, not a dnsmasq.d file, so the
// hookup rung has to look for the rule rather than report a macOS path.
func TestResolverHookupLooksForTheNRPTRule(t *testing.T) {
	prev := nrptRuleExists
	t.Cleanup(func() { nrptRuleExists = prev })

	var asked string
	nrptRuleExists = func(ns string) bool { asked = ns; return true }
	kind, exists, path := defaultResolverHookup()
	if kind != windowsNRPTKind || !exists || path != nrptNamespace() || asked != nrptNamespace() {
		t.Errorf("defaultResolverHookup() = %q %v %q (asked %q)", kind, exists, path, asked)
	}

	nrptRuleExists = func(string) bool { return false }
	if _, exists, _ := defaultResolverHookup(); exists {
		t.Error("a missing NRPT rule should not count as a hookup")
	}
}
