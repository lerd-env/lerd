package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// stopRecorder records Stop calls so reconcileStaleFrankenPHP's teardown can be
// asserted without touching real systemd.
type stopRecorder struct{ stopped []string }

func (s *stopRecorder) Start(string) error                { return nil }
func (s *stopRecorder) Stop(name string) error            { s.stopped = append(s.stopped, name); return nil }
func (s *stopRecorder) Restart(string) error              { return nil }
func (s *stopRecorder) UnitStatus(string) (string, error) { return "inactive", nil }
func (s *stopRecorder) AllUnitStates() map[string]string  { return nil }

// stubFrankenPHPTeardown routes the podman teardown calls through the recorder
// and a no-op daemon-reload, restoring the originals on cleanup.
func stubFrankenPHPTeardown(t *testing.T, lc *stopRecorder) {
	t.Helper()
	origLC := podman.UnitLifecycle
	origReload := podman.DaemonReloadFn
	origRemoveUnit := podman.RemoveContainerUnitFn
	podman.UnitLifecycle = lc
	podman.DaemonReloadFn = func() error { return nil }
	podman.RemoveContainerUnitFn = nil
	t.Cleanup(func() {
		podman.UnitLifecycle = origLC
		podman.DaemonReloadFn = origReload
		podman.RemoveContainerUnitFn = origRemoveUnit
	})
}

func writeFakeFPQuadlet(t *testing.T, siteName string) string {
	t.Helper()
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, podman.FrankenPHPContainerName(siteName)+".container")
	if err := os.WriteFile(path, []byte("[Container]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestReconcileStaleFrankenPHP covers the orphan a FrankenPHP->FPM re-link
// leaves behind: the per-site fp quadlet is WantedBy=default.target with
// Restart=always, so podman's generator keeps auto-starting a container that
// lerd start/stop never enumerate. The reconcile must remove it only when the
// site is no longer FrankenPHP, and never touch a still-FrankenPHP site.
func TestReconcileStaleFrankenPHP(t *testing.T) {
	t.Run("removes stale quadlet when site is no longer frankenphp", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		rec := &stopRecorder{}
		stubFrankenPHPTeardown(t, rec)

		path := writeFakeFPQuadlet(t, "scorediviner")
		reconcileStaleFrankenPHP(config.Site{Name: "scorediviner"})

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("stale fp quadlet should be removed, stat err = %v", err)
		}
		if len(rec.stopped) != 1 || rec.stopped[0] != "lerd-fp-scorediviner" {
			t.Errorf("stopped = %v, want [lerd-fp-scorediviner]", rec.stopped)
		}
	})

	t.Run("keeps quadlet when site is still frankenphp", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		rec := &stopRecorder{}
		stubFrankenPHPTeardown(t, rec)

		path := writeFakeFPQuadlet(t, "personal")
		reconcileStaleFrankenPHP(config.Site{Name: "personal", Runtime: "frankenphp"})

		if _, err := os.Stat(path); err != nil {
			t.Errorf("frankenphp quadlet should be kept, stat err = %v", err)
		}
		if len(rec.stopped) != 0 {
			t.Errorf("stopped = %v, want none (site still frankenphp)", rec.stopped)
		}
	})

	t.Run("no-op when no fp quadlet exists", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		rec := &stopRecorder{}
		stubFrankenPHPTeardown(t, rec)

		reconcileStaleFrankenPHP(config.Site{Name: "freshapp"})
		if len(rec.stopped) != 0 {
			t.Errorf("stopped = %v, want none (no quadlet)", rec.stopped)
		}
	})
}

func writeFakeCFPMQuadlet(t *testing.T, siteName string) string {
	t.Helper()
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, podman.CustomFPMContainerName(siteName)+".container")
	if err := os.WriteFile(path, []byte("[Container]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestReconcileStaleCustomFPM covers the mirror of the FrankenPHP orphan: a
// custom-FPM site re-linked as plain FPM (or a reverse-proxied container) leaves
// its per-site lerd-cfpm-<site> quadlet auto-starting. The reconcile removes it
// only when the site is no longer fpm-custom, and never touches one still custom.
func TestReconcileStaleCustomFPM(t *testing.T) {
	t.Run("removes stale quadlet when site is no longer custom-fpm", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		rec := &stopRecorder{}
		stubFrankenPHPTeardown(t, rec)

		path := writeFakeCFPMQuadlet(t, "shopapp")
		reconcileStaleCustomFPM(config.Site{Name: "shopapp"})

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("stale cfpm quadlet should be removed, stat err = %v", err)
		}
		if len(rec.stopped) != 1 || rec.stopped[0] != podman.CustomFPMContainerName("shopapp") {
			t.Errorf("stopped = %v, want [%s]", rec.stopped, podman.CustomFPMContainerName("shopapp"))
		}
	})

	t.Run("keeps quadlet when site is still custom-fpm", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		rec := &stopRecorder{}
		stubFrankenPHPTeardown(t, rec)

		path := writeFakeCFPMQuadlet(t, "billing")
		reconcileStaleCustomFPM(config.Site{Name: "billing", Runtime: "fpm-custom"})

		if _, err := os.Stat(path); err != nil {
			t.Errorf("custom-fpm quadlet should be kept, stat err = %v", err)
		}
		if len(rec.stopped) != 0 {
			t.Errorf("stopped = %v, want none (site still custom-fpm)", rec.stopped)
		}
	})

	t.Run("no-op when no cfpm quadlet exists", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		rec := &stopRecorder{}
		stubFrankenPHPTeardown(t, rec)

		reconcileStaleCustomFPM(config.Site{Name: "freshapp"})
		if len(rec.stopped) != 0 {
			t.Errorf("stopped = %v, want none (no quadlet)", rec.stopped)
		}
	})
}
