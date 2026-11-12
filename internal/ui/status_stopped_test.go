package ui

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The dashboard reads this to leave a stack the user stopped alone instead of
// auto-starting it again on the next page load.
func TestStatusReportsAStopTheUserAskedFor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if buildStatus().Stopped {
		t.Fatal("stopped before any stop")
	}
	if err := config.MarkStopped(); err != nil {
		t.Fatal(err)
	}
	if !buildStatus().Stopped {
		t.Error("status does not report the stop")
	}
	if err := config.ClearStopped(); err != nil {
		t.Fatal(err)
	}
	if buildStatus().Stopped {
		t.Error("status still reports a stop after start cleared it")
	}
}
