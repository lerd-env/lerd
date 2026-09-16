package cli

import (
	"slices"
	"strings"
	"testing"
)

// The state was only writable, never readable: enable/disable existed with no way
// to ask which one is in force.
func TestAutostartStatusLine(t *testing.T) {
	if got := autostartStatusLine(true); !strings.Contains(got, "enabled") {
		t.Errorf("enabled status = %q", got)
	}
	if got := autostartStatusLine(false); !strings.Contains(got, "disabled") {
		t.Errorf("disabled status = %q", got)
	}
}

func TestAutostartHasStatusSubcommand(t *testing.T) {
	var names []string
	for _, c := range NewAutostartCmd().Commands() {
		names = append(names, c.Name())
	}
	for _, want := range []string{"enable", "disable", "status"} {
		if !slices.Contains(names, want) {
			t.Errorf("autostart is missing %q, has %v", want, names)
		}
	}
}
