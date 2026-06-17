package siteops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// recordingLifecycle records which units were stopped so the demote test can
// assert the FrankenPHP container is actually torn down.
type recordingLifecycle struct{ stopped map[string]bool }

func (r *recordingLifecycle) Start(string) error { return nil }
func (r *recordingLifecycle) Stop(name string) error {
	if r.stopped == nil {
		r.stopped = map[string]bool{}
	}
	r.stopped[name] = true
	return nil
}
func (r *recordingLifecycle) Restart(string) error              { return nil }
func (r *recordingLifecycle) UnitStatus(string) (string, error) { return "inactive", nil }
func (r *recordingLifecycle) AllUnitStates() map[string]string  { return map[string]string{} }

// fakePodmanOnPath drops a no-op `podman` binary early on PATH so calls that
// shell out (nginx.Reload runs `podman exec lerd-nginx ...`) succeed.
func fakePodmanOnPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "podman"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Demoting a FrankenPHP site to FPM must stop the per-site FrankenPHP container
// (so it is not orphaned) and must stop the site's workers before teardown then
// recreate them against the shared FPM container after the registry is flipped.
func TestDemoteFrankenPHPToFPM_stopsContainerAndRecreatesWorkers(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	fakePodmanOnPath(t)

	rec := &recordingLifecycle{}
	origLC := podman.UnitLifecycle
	podman.UnitLifecycle = rec
	origDR := podman.DaemonReloadFn
	podman.DaemonReloadFn = func() error { return nil }
	origStop := StopRuntimeWorkers
	origRecreate := RecreateFPMWorkers
	t.Cleanup(func() {
		podman.UnitLifecycle = origLC
		podman.DaemonReloadFn = origDR
		StopRuntimeWorkers = origStop
		RecreateFPMWorkers = origRecreate
	})

	var events []string
	var recreatedWorkers []string
	var runtimeAtRecreate = "unset"
	captured := []string{"queue", "schedule"}
	StopRuntimeWorkers = func(*config.Site) []string {
		events = append(events, "stop")
		return captured
	}
	RecreateFPMWorkers = func(s *config.Site, workers []string) {
		events = append(events, "recreate")
		recreatedWorkers = workers
		runtimeAtRecreate = s.Runtime
	}

	projectDir := filepath.Join(tmp, "app")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	site := config.Site{
		Name:       "app",
		Domains:    []string{"app.test"},
		Path:       projectDir,
		PHPVersion: "8.1",
		Runtime:    "frankenphp",
	}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}

	s := site
	if err := DemoteFrankenPHPToFPM(&s); err != nil {
		t.Fatalf("DemoteFrankenPHPToFPM: %v", err)
	}

	if len(events) != 2 || events[0] != "stop" || events[1] != "recreate" {
		t.Fatalf("hook order = %v, want [stop recreate]", events)
	}
	if wantUnit := podman.FrankenPHPContainerName("app"); !rec.stopped[wantUnit] {
		t.Errorf("FrankenPHP container not stopped: stopped=%v want %q", rec.stopped, wantUnit)
	}
	if runtimeAtRecreate != "" {
		t.Errorf("runtime at recreate = %q, want empty (registry already flipped to FPM)", runtimeAtRecreate)
	}
	if len(recreatedWorkers) != len(captured) {
		t.Errorf("recreated workers = %v, want the captured set %v", recreatedWorkers, captured)
	}
	got, err := config.FindSite("app")
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtime != "" || got.RuntimeWorker {
		t.Errorf("registry still on FrankenPHP: runtime=%q worker=%v", got.Runtime, got.RuntimeWorker)
	}
}
