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

func TestHideStreamingDatabasesDropsAHiddenSitesDatabases(t *testing.T) {
	domains := map[string]bool{"secret.test": true}
	in := []dbEngineResponse{{Service: "mysql", Databases: []dbEntryResponse{
		{Name: "secret", Site: "secret.test"},
		{Name: "secret_feat_x", Site: "feat-x.secret.test", Branch: "feat-x"},
		{Name: "open", Site: "open.test"},
		{Name: "scratch"},
		{Name: "secret_testing"},
	}}}
	out := hideStreamingDatabases(in, map[string]bool{"secret": true}, domains)
	var names []string
	for _, db := range out[0].Databases {
		names = append(names, db.Name)
	}
	if len(names) != 2 || names[0] != "open" || names[1] != "scratch" {
		t.Fatalf("databases = %v, want open and scratch", names)
	}
}

func TestHideStreamingEntityRowsDropsAHiddenSitesBuckets(t *testing.T) {
	domains := map[string]bool{"secret.test": true}
	in := []entityKindResponse{{Kind: "buckets", Rows: []entityRowResponse{
		{Name: "secret-media", Site: "secret.test"}, {Name: "open-media", Site: "open.test"}, {Name: "loose"}, {Name: "secret-uploads"},
	}}}
	out := hideStreamingEntityRows(in, map[string]bool{"secret": true}, domains)
	if len(out[0].Rows) != 2 || out[0].Rows[0].Name != "open-media" || out[0].Rows[1].Name != "loose" {
		t.Fatalf("rows = %+v, want open-media and loose", out[0].Rows)
	}
}

func TestHideStreamingAutoSnapshotDropsAHiddenSite(t *testing.T) {
	in := autoSnapshotResponse{Sites: []autoSnapshotSiteStatus{{Site: "secret"}, {Site: "open"}}}
	out := hideStreamingAutoSnapshot(in, map[string]bool{"secret": true})
	if len(out.Sites) != 1 || out.Sites[0].Site != "open" {
		t.Fatalf("sites = %+v, want open only", out.Sites)
	}
}

func TestHideStreamingServicesDropsAHiddenWorktreesDomain(t *testing.T) {
	out := hideStreamingServices([]ServiceResponse{{Name: "mysql", SiteDomains: []string{"feat-x.secret.test", "open.test"}}},
		map[string]bool{"secret": true}, map[string]bool{"secret.test": true})
	if len(out[0].SiteDomains) != 1 || out[0].SiteDomains[0] != "open.test" {
		t.Fatalf("site_domains = %v, want open.test", out[0].SiteDomains)
	}
}
