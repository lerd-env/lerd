package cli

import (
	"sort"
	"testing"

	"github.com/geodro/lerd/internal/workerheal"
)

// A unit whose worktree is gone can never start again, so pruning is the only
// resolution. Everything else in the same batch, including an ordinary failure
// that heal can still fix, must be left exactly as it is.
func TestPruneOrphanedWorkerUnits_removesOnlyOrphans(t *testing.T) {
	registerSite(t, "ws", "/p/ws")
	fake := &stopTrackingMgr{}
	swapMgr(t, fake)

	pruned := PruneOrphanedWorkerUnits([]UnhealthyWorker{
		{Site: "ws", Worker: "vite", Unit: "lerd-vite-ws-feat-x", State: workerheal.StateOrphaned},
		{Site: "ws", Worker: "queue", Unit: "lerd-queue-ws", State: "failed"},
		{Site: "ws", Worker: "vite", Unit: "lerd-vite-ws-feat-y", State: workerheal.StateOrphaned},
		{Site: "ws", Worker: "reverb", Unit: "lerd-reverb-ws", State: "unreachable"},
	})

	if pruned != 2 {
		t.Errorf("pruned = %d, want 2", pruned)
	}
	got := append([]string(nil), fake.removeServiceCalls...)
	sort.Strings(got)
	want := []string{"lerd-vite-ws-feat-x", "lerd-vite-ws-feat-y"}
	if !equalStrings(got, want) {
		t.Errorf("removeServiceCalls = %v, want %v", got, want)
	}
	for _, unit := range got {
		found := false
		for _, d := range fake.disableCalls {
			if d == unit {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected Disable(%q), saw %v", unit, fake.disableCalls)
		}
	}
}

// Nothing orphaned means nothing touched, so a steady daemon tick over a
// healthy install never disables or removes a single unit.
func TestPruneOrphanedWorkerUnits_noOrphansTouchesNothing(t *testing.T) {
	registerSite(t, "ws", "/p/ws")
	fake := &stopTrackingMgr{}
	swapMgr(t, fake)

	if pruned := PruneOrphanedWorkerUnits([]UnhealthyWorker{
		{Site: "ws", Worker: "queue", Unit: "lerd-queue-ws", State: "failed"},
	}); pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}
	if len(fake.removeServiceCalls) != 0 || len(fake.disableCalls) != 0 {
		t.Errorf("nothing should be touched, got removes=%v disables=%v",
			fake.removeServiceCalls, fake.disableCalls)
	}
}

func TestPruneOrphanedWorkerUnits_emptyInput(t *testing.T) {
	fake := &stopTrackingMgr{}
	swapMgr(t, fake)
	if pruned := PruneOrphanedWorkerUnits(nil); pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}
}
