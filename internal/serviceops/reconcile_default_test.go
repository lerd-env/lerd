package serviceops

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/registry"
)

// defaultSeams drives ReconcileDefaultService: whether the unit is installed,
// whether rendering it changed it, and a running container started at boot.
func defaultSeams(t *testing.T, installed, unitChanged bool) *[]string {
	t.Helper()
	var calls []string
	prev := ensureDefaultQuadletFn
	t.Cleanup(func() { ensureDefaultQuadletFn = prev })
	ensureDefaultQuadletFn = func(name string) (bool, error) {
		calls = append(calls, "regenerate "+name)
		return unitChanged, nil
	}
	boot := time.Unix(1_000_000, 0)
	t.Cleanup(swapDriftSeams(t,
		func(*config.CustomService) error { return nil },
		func(*config.CustomService) (time.Time, bool) { return time.Time{}, false },
		func(string) (time.Time, bool) { return boot, true },
		func(name string) error { calls = append(calls, "restart "+name); return nil },
		func(string) bool { return installed },
	))
	return &calls
}

// A binary upgrade that changes a default preset's unit (mysql's start command)
// left the installed unit as it was, because lerd start only checked config files.
func TestReconcileDefaultService_rewritesAndRestartsAChangedUnit(t *testing.T) {
	reconcileEnv(t)
	calls := defaultSeams(t, true, true)

	restarted, err := ReconcileDefaultService("mysql")
	if err != nil {
		t.Fatal(err)
	}
	if !restarted || !slices.Equal(*calls, []string{"regenerate mysql", "restart lerd-mysql"}) {
		t.Fatalf("restarted = %v, calls = %v, want the unit rewritten then restarted", restarted, *calls)
	}
}

func TestReconcileDefaultService_leavesAnUninstalledServiceAlone(t *testing.T) {
	reconcileEnv(t)
	calls := defaultSeams(t, false, true)

	if restarted, err := ReconcileDefaultService("mysql"); err != nil || restarted || len(*calls) != 0 {
		t.Fatalf("restarted = %v, err = %v, calls = %v, want nothing for a service that is not installed", restarted, err, *calls)
	}
}

func TestReconcileDefaultService_noRestartWhenTheUnitIsCurrent(t *testing.T) {
	reconcileEnv(t)
	calls := defaultSeams(t, true, false)

	if restarted, err := ReconcileDefaultService("mysql"); err != nil || restarted || slices.Contains(*calls, "restart lerd-mysql") {
		t.Fatalf("restarted = %v, err = %v, calls = %v, want no restart in a steady state", restarted, err, *calls)
	}
}

// The refresh on start must not double as an upgrade: with no image recorded in
// config, resolving the preset afresh picked the newest minor from the registry.
func TestEnsureDefaultPresetQuadletDiff_keepsTheInstalledImage(t *testing.T) {
	reconcileEnv(t)
	cacheDir := t.TempDir()
	t.Setenv("LERD_REGISTRY_CACHE_DIR", cacheDir)
	const installed = "docker.io/library/mysql:8.4.1"
	ref, err := registry.ParseImage(installed)
	if err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte(ref.Registry + "/" + ref.Repo))
	tags := `{"fetched_at":"` + time.Now().UTC().Format(time.RFC3339) + `","tags":[{"name":"8.4.1"},{"name":"8.4.13"}]}`
	if err := os.WriteFile(filepath.Join(cacheDir, hex.EncodeToString(key[:])+".json"), []byte(tags), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.QuadletDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	unit := filepath.Join(config.QuadletDir(), "lerd-mysql.container")
	if err := os.WriteFile(unit, []byte("[Container]\nImage="+installed+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ensureDefaultPresetQuadletDiff("mysql"); err != nil {
		t.Fatal(err)
	}
	if got := podman.InstalledImage("lerd-mysql"); got != installed {
		t.Fatalf("lerd start moved mysql to %s, want it left on %s", got, installed)
	}
}
