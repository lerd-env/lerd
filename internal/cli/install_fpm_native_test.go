package cli

import (
	"strings"
	"testing"
)

// Install started an FPM container for every installed PHP version without
// asking where PHP runs, so a native install brought up containers nothing
// serves from. Their quadlet restarts always, so once up they came back for
// good and the runtime switch's teardown was undone by the next install.
func TestFPMUnitsToSettle(t *testing.T) {
	versions := []string{"8.3", "8.4"}

	t.Run("the container runtime starts them", func(t *testing.T) {
		start, stop := fpmVersionsToSettle(versions, true)
		if got := strings.Join(start, ","); got != "8.3,8.4" {
			t.Errorf("start = %q, want both versions", got)
		}
		if len(stop) != 0 {
			t.Errorf("stop = %v, want nothing", stop)
		}
	})

	t.Run("the native runtime stops them instead", func(t *testing.T) {
		start, stop := fpmVersionsToSettle(versions, false)
		if len(start) != 0 {
			t.Errorf("start = %v, want nothing on the native runtime", start)
		}
		// Stopped rather than merely left alone: an install that predates this
		// has them running, and declining to start one already up changes
		// nothing on the machine that has the problem.
		if got := strings.Join(stop, ","); got != "8.3,8.4" {
			t.Errorf("stop = %q, want both versions", got)
		}
	})

	t.Run("no versions is nothing either way", func(t *testing.T) {
		for _, wanted := range []bool{true, false} {
			if start, stop := fpmVersionsToSettle(nil, wanted); len(start) != 0 || len(stop) != 0 {
				t.Errorf("wanted=%v gave start=%v stop=%v, want nothing", wanted, start, stop)
			}
		}
	})
}
