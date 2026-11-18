package cli

import (
	"strings"
	"testing"
)

// lan:status said loopback-only on macOS while lerd-nginx answered the whole
// network on 80 and 443, because gvproxy can only publish those wide.
func TestLANWideBindSummaryNamesReachableUnits(t *testing.T) {
	got := lanWideBindSummary(map[string][]int{
		"lerd-nginx":  {80, 443},
		"lerd-custom": {25},
	}, false)

	for _, want := range []string{"lerd-custom (25)", "lerd-nginx (80, 443)", "every interface"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q: %s", want, got)
		}
	}
	if strings.Index(got, "lerd-custom") > strings.Index(got, "lerd-nginx") {
		t.Errorf("units should be listed in a stable order: %s", got)
	}
}

func TestLANWideBindSummarySilentWhenExposedOrNothingWidened(t *testing.T) {
	if got := lanWideBindSummary(map[string][]int{"lerd-nginx": {80}}, true); got != "" {
		t.Errorf("an exposed lerd binds wide on purpose, got %q", got)
	}
	if got := lanWideBindSummary(map[string][]int{}, false); got != "" {
		t.Errorf("nothing widened should say nothing, got %q", got)
	}
}
