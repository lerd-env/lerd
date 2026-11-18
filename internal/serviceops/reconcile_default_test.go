package serviceops

import (
	"slices"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// defaultSeams drives ReconcileDefaultService: whether the unit is installed,
// whether rendering it changed it, and a running container started at boot.
func defaultSeams(t *testing.T, installed, unitChanged bool) *[]string {
	t.Helper()
	var calls []string
	prev := ensureDefaultQuadletFn
	t.Cleanup(func() { ensureDefaultQuadletFn = prev })
	ensureDefaultQuadletFn = func(name string) (bool, error) {
		calls = append(calls, "regenerate "+name)
		return unitChanged, nil
	}
	boot := time.Unix(1_000_000, 0)
	t.Cleanup(swapDriftSeams(t,
		func(*config.CustomService) error { return nil },
		func(*config.CustomService) (time.Time, bool) { return time.Time{}, false },
		func(string) (time.Time, bool) { return boot, true },
		func(name string) error { calls = append(calls, "restart "+name); return nil },
		func(string) bool { return installed },
	))
	return &calls
}

// A binary upgrade that changes a default preset's unit (mysql's start command)
// left the installed unit as it was, because lerd start only checked config files.
func TestReconcileDefaultService_rewritesAndRestartsAChangedUnit(t *testing.T) {
	reconcileEnv(t)
	calls := defaultSeams(t, true, true)

	restarted, err := ReconcileDefaultService("mysql")
	if err != nil {
		t.Fatal(err)
	}
	if !restarted || !slices.Equal(*calls, []string{"regenerate mysql", "restart lerd-mysql"}) {
		t.Fatalf("restarted = %v, calls = %v, want the unit rewritten then restarted", restarted, *calls)
	}
}

func TestReconcileDefaultService_leavesAnUninstalledServiceAlone(t *testing.T) {
	reconcileEnv(t)
	calls := defaultSeams(t, false, true)

	if restarted, err := ReconcileDefaultService("mysql"); err != nil || restarted || len(*calls) != 0 {
		t.Fatalf("restarted = %v, err = %v, calls = %v, want nothing for a service that is not installed", restarted, err, *calls)
	}
}

func TestReconcileDefaultService_noRestartWhenTheUnitIsCurrent(t *testing.T) {
	reconcileEnv(t)
	calls := defaultSeams(t, true, false)

	if restarted, err := ReconcileDefaultService("mysql"); err != nil || restarted || slices.Contains(*calls, "restart lerd-mysql") {
		t.Fatalf("restarted = %v, err = %v, calls = %v, want no restart in a steady state", restarted, err, *calls)
	}
}
