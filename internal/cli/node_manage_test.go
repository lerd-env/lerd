package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/services"
)

// recordingMgr embeds the real ServiceManager interface (nil here, since only
// the installed checks are exercised on the path under test) and records which
// unit names regenerateWorkerUnit probes via IsEnabled. Both checks returning
// false makes regenerateWorkerUnit return early, so no heavier lifecycle method
// runs.
type recordingMgr struct {
	services.ServiceManager
	enabledProbed map[string]bool
}

func (m *recordingMgr) IsEnabled(name string) bool {
	m.enabledProbed[name] = true
	return false
}

func (m *recordingMgr) IsActive(string) bool { return false }

// RegenerateHostWorkersForSite must not resurrect a host worker the idle engine
// has suspended: restarting it runs ClearIdleSuspendOnStart, which drops it from
// the suspended list so the engine can no longer see it running and it stays up
// forever on an idle site (the live "vite up while site idle after install" bug).
// The probe is whether regenerateWorkerUnit was even entered for the suspended
// worker — it must be skipped before the IsEnabled check.
func TestRegenerateHostWorkersForSite_skipsIdleSuspended(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "site")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: laravel\n"), 0644); err != nil {
		t.Fatal(err)
	}
	proj, err := config.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj.CustomWorkers = map[string]config.FrameworkWorker{
		"vite": {Command: "npm run dev", Host: true},
		"echo": {Command: "npm run echo", Host: true},
	}
	if err := config.SaveProjectConfig(dir, proj); err != nil {
		t.Fatal(err)
	}

	rec := &recordingMgr{enabledProbed: map[string]bool{}}
	prev := services.Mgr
	services.Mgr = rec
	t.Cleanup(func() { services.Mgr = prev })

	// vite is idle-suspended; echo is not. Only echo should be regenerated.
	site := config.Site{
		Name: "myapp", Path: dir, PHPVersion: "8.4", Framework: "laravel",
		IdleSuspendedWorkers: []string{"vite"},
	}
	RegenerateHostWorkersForSite(site)

	if rec.enabledProbed["lerd-vite-myapp"] {
		t.Error("suspended host worker vite was regenerated; it must be skipped to stay asleep")
	}
	if !rec.enabledProbed["lerd-echo-myapp"] {
		t.Error("non-suspended host worker echo should still be regenerated")
	}
}
