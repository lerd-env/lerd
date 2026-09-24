package ui

import (
	"testing"

	"github.com/geodro/lerd/internal/stats"
)

func TestHideStreamingServicesDropsAHiddenSitesWorkers(t *testing.T) {
	hidden := map[string]bool{"secret": true}
	domains := map[string]bool{"secret.test": true}
	in := []ServiceResponse{
		{Name: "mysql", SiteCount: 2, SiteDomains: []string{"open.test", "secret.test"}},
		{Name: "vite-secret", WorkerSite: "secret"},
		{Name: "queue-secret", QueueSite: "secret"},
		{Name: "horizon-secret", HorizonSite: "secret"},
		{Name: "vite-open", WorkerSite: "open"},
	}
	out := hideStreamingServices(in, hidden, domains)
	if len(out) != 2 || out[0].Name != "mysql" || out[1].Name != "vite-open" {
		t.Fatalf("got %+v, want mysql and vite-open only", out)
	}
	if len(out[0].SiteDomains) != 1 || out[0].SiteDomains[0] != "open.test" {
		t.Errorf("site_domains = %v, want only open.test", out[0].SiteDomains)
	}
}

func TestHideStreamingServicesLeavesEverythingWhenNothingIsHidden(t *testing.T) {
	in := []ServiceResponse{{Name: "vite-open", WorkerSite: "open"}}
	if out := hideStreamingServices(in, map[string]bool{}, map[string]bool{}); len(out) != 1 {
		t.Fatalf("got %+v, want the list untouched", out)
	}
}

func TestHideStreamingContainersDropsAHiddenSitesContainers(t *testing.T) {
	snap := stats.Snapshot{Containers: []stats.ContainerStat{
		{Name: "lerd-mysql"}, {Name: "lerd-vite-secret"}, {Name: "lerd-vite-secret-feat-x"}, {Name: "lerd-vite-open"},
	}}
	out := hideStreamingContainers(snap, map[string]bool{"secret": true})
	if len(out.Containers) != 2 || out.Containers[0].Name != "lerd-mysql" || out.Containers[1].Name != "lerd-vite-open" {
		t.Fatalf("got %+v, want lerd-mysql and lerd-vite-open only", out.Containers)
	}
}
