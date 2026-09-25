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

// on and off are what idle, dump and streaming take, so autostart answers to
// them too instead of printing its help and exiting 0.
func TestAutostartAcceptsOnAndOff(t *testing.T) {
	cmd := NewAutostartCmd()
	for _, pair := range [][2]string{{"on", "enable"}, {"off", "disable"}} {
		sub, _, err := cmd.Find([]string{pair[0]})
		if err != nil || sub.Name() != pair[1] {
			t.Errorf("autostart %s resolved to %v (err %v), want %s", pair[0], sub, err, pair[1])
		}
	}
}
