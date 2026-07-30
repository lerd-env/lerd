package tui

import (
	"runtime"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The worker-mode row should only be present on macOS so the Linux
// settings overlay doesn't advertise a setting that has no effect.
func TestSettingsRows_WorkerModeVisibilityMatchesPlatform(t *testing.T) {
	m := NewModel("test")
	rows := m.settingsRows()

	var found bool
	for _, r := range rows {
		if r.kind == settingsWorkerMode {
			found = true
			break
		}
	}

	wantPresent := runtime.GOOS == "darwin"
	if found != wantPresent {
		t.Errorf("worker-mode row present=%v on %s, want present=%v",
			found, runtime.GOOS, wantPresent)
	}
}

func TestSettingsRowsReflectManagedServiceLANExposure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.LAN.ServicesExposed = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	rows := NewModel("test").settingsRows()
	for _, row := range rows {
		if row.kind == settingsLANServices {
			if !row.on {
				t.Fatal("managed-service LAN row did not reflect enabled config")
			}
			if row.label != "Managed service LAN access (inactive — LAN exposure off)" {
				t.Fatalf("row label = %q, want inactive state", row.label)
			}
			cfg.LAN.Exposed = true
			if got := managedServiceLANLabel(cfg); got != "Managed service LAN access" {
				t.Fatalf("active label = %q", got)
			}
			return
		}
	}
	t.Fatal("managed-service LAN row is missing")
}
