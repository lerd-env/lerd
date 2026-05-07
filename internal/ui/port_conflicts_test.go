package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPortConflictsFor_emptySsOutput verifies the helper short-circuits when
// the listening-port listing is unavailable (e.g. ss/lsof failed). Stopped
// services should not be flagged as conflicting just because we couldn't
// scan the host.
func TestPortConflictsFor_emptySsOutput(t *testing.T) {
	if got := portConflictsFor("lerd-postgres", ""); got != nil {
		t.Errorf("expected nil for empty ssOutput, got %+v", got)
	}
}

// TestPortConflictsFor_unknownUnit covers the case where the unit name
// matches no installed service. CollectPortChecks should return no entries
// and the helper should return nil rather than scanning unrelated ports.
func TestPortConflictsFor_unknownUnit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	ss := "LISTEN 0 128 0.0.0.0:5432 0.0.0.0:*\n"
	if got := portConflictsFor("lerd-not-a-real-service", ss); got != nil {
		t.Errorf("expected nil for unknown unit, got %+v", got)
	}
}

// TestPortConflictsFor_customService validates the end-to-end path for a
// custom service: a YAML file in CustomServicesDir advertising port 5432,
// against an ssOutput containing :5432, must surface a single conflict
// labelled with the service name.
func TestPortConflictsFor_customService(t *testing.T) {
	tmp := t.TempDir()
	cfgHome := filepath.Join(tmp, "config")
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	servicesDir := filepath.Join(cfgHome, "lerd", "services")
	if err := os.MkdirAll(servicesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yaml := "name: myservice\nimage: example/myservice:latest\nports:\n  - \"5432:5432\"\n"
	if err := os.WriteFile(filepath.Join(servicesDir, "myservice.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	ss := "LISTEN 0 128 0.0.0.0:5432 0.0.0.0:*\n"
	got := portConflictsFor("lerd-myservice", ss)
	if len(got) != 1 {
		t.Fatalf("expected 1 conflict, got %d (%+v)", len(got), got)
	}
	if got[0].Port != "5432" {
		t.Errorf("port = %q, want 5432", got[0].Port)
	}
	if got[0].Label != "myservice" {
		t.Errorf("label = %q, want myservice", got[0].Label)
	}

	// No conflict when the same port is absent from the listing.
	if extra := portConflictsFor("lerd-myservice", "LISTEN 0 128 0.0.0.0:9999 0.0.0.0:*\n"); extra != nil {
		t.Errorf("expected no conflict when port not listening, got %+v", extra)
	}
}
