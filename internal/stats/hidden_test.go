package stats

import "testing"

func TestWithoutSitesDropsASitesContainers(t *testing.T) {
	snap := Snapshot{Containers: []ContainerStat{
		{Name: "lerd-mysql"}, {Name: "lerd-vite-secret"}, {Name: "lerd-vite-secret-feat-x"}, {Name: "lerd-vite-open"},
	}}
	out := WithoutSites(snap, map[string]bool{"secret": true})
	if len(out.Containers) != 2 || out.Containers[0].Name != "lerd-mysql" || out.Containers[1].Name != "lerd-vite-open" {
		t.Fatalf("got %+v, want lerd-mysql and lerd-vite-open only", out.Containers)
	}
}
