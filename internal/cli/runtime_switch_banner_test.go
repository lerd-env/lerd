package cli

import (
	"strings"
	"testing"
)

// During a switch every red row is expected and temporary. The reader has to be
// told that before being told to repair anything.
func TestRuntimeSwitchBanner(t *testing.T) {
	if got := runtimeSwitchBanner(false); got != "" {
		t.Errorf("no switch running should print nothing, got %q", got)
	}
	got := runtimeSwitchBanner(true)
	if !strings.Contains(strings.ToLower(got), "switch") {
		t.Errorf("banner should name the switch, got %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "again") && !strings.Contains(strings.ToLower(got), "finish") {
		t.Errorf("banner should tell the reader to wait, got %q", got)
	}
}
