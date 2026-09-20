package config

import "testing"

// A service records what its preset declared at install time, so one installed
// before the preset gained a dashboard has none of its own. The store reaches
// every install within a day and a schema tree can add a dashboard to a
// definition an older binary was served without, so the preset is the answer
// when the record is silent. Otherwise upgrading lerd leaves the service exactly
// as installed and the dashboard never appears.
func TestServiceDashboard_FallsBackToThePreset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := SaveStorePreset("solr", []byte("name: solr\nimage: docker.io/library/solr:9\ndashboard: http://localhost:8983/solr/\n")); err != nil {
		t.Fatal(err)
	}

	installed := &CustomService{Name: "solr", Preset: "solr"}
	if got := ServiceDashboard(installed); got != "http://localhost:8983/solr/" {
		t.Errorf("a record with no dashboard should take the preset's, got %q", got)
	}

	own := &CustomService{Name: "solr", Preset: "solr", Dashboard: "http://localhost:9999/own/"}
	if got := ServiceDashboard(own); got != "http://localhost:9999/own/" {
		t.Errorf("a record with its own dashboard keeps it, got %q", got)
	}

	if got := ServiceDashboard(&CustomService{Name: "plain"}); got != "" {
		t.Errorf("a service with no preset and no dashboard has none, got %q", got)
	}
	if got := ServiceDashboard(nil); got != "" {
		t.Errorf("nil service has no dashboard, got %q", got)
	}
}
