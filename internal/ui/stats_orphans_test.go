package ui

import (
	"testing"

	"github.com/geodro/lerd/internal/stats"
)

func TestMarkOrphans_flagsOnlyServiceContainersNothingStandsBehind(t *testing.T) {
	snap := stats.Snapshot{Containers: []stats.ContainerStat{
		{Name: "lerd-phpmyadmin"}, {Name: "lerd-mysql"}, {Name: "other"},
	}}
	got := markOrphans(snap, func(name string) bool { return name == "phpmyadmin" })
	for i, want := range []bool{true, false, false} {
		if got.Containers[i].Orphaned != want {
			t.Errorf("%s orphaned = %v, want %v", got.Containers[i].Name, got.Containers[i].Orphaned, want)
		}
	}
	if snap.Containers[0].Orphaned {
		t.Error("marked the shared cached snapshot in place")
	}
}
