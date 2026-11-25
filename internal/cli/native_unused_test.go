package cli

import (
	"errors"
	"slices"
	"testing"
)

// Under the native runtime pausing or unlinking the last site on a version
// stops its pool, as the container runtime stops the FPM. One that will not stop
// must not keep the rest running.
func TestStopUnusedNativePools(t *testing.T) {
	var stopped []string
	stopUnusedNativePools([]string{"8.1", "8.2", "8.3", "8.4", "8.5"},
		map[string]bool{"8.4": true, "8.5": true},
		func(v string) bool { return v != "8.2" },
		func(v string) error {
			stopped = append(stopped, v)
			if v == "8.1" {
				return errors.New("bootout failed")
			}
			return nil
		})
	if !slices.Equal(stopped, []string{"8.1", "8.3"}) {
		t.Errorf("stopped %v, want the running unused 8.1 and 8.3", stopped)
	}
}
