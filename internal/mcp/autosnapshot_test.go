package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

// autoSnapshotEnv points config at temp dirs and registers one site whose .env
// names a lerd engine.
func autoSnapshotEnv(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := t.TempDir()
	env := "DB_CONNECTION=mysql\nDB_HOST=lerd-mysql\nDB_DATABASE=shop_db\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{
		{Name: "shop", Domains: []string{"shop.test"}, Path: dir, PHPVersion: "8.4"},
	}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}
}

func TestExecDBAutoSet_policyAndSiteOverride(t *testing.T) {
	autoSnapshotEnv(t)

	res, rpcErr := execDBAutoSet(map[string]any{"enabled": true, "every": "6h", "keep": float64(5)})
	if rpcErr != nil {
		t.Fatalf("auto_set: %v", rpcErr)
	}
	if !strings.Contains(toolText(res), `"enabled": true`) {
		t.Fatalf("policy not reported back: %s", toolText(res))
	}
	cfg, _ := config.LoadGlobal()
	if !cfg.AutoSnapshotEnabled() || cfg.AutoSnapshotEvery() != 6*time.Hour || cfg.AutoSnapshotKeep() != 5 {
		t.Fatalf("policy not saved: %+v", cfg.AutoSnapshot)
	}

	if _, rpcErr = execDBAutoSet(map[string]any{"site": "shop", "mode": "off"}); rpcErr != nil {
		t.Fatalf("site override: %v", rpcErr)
	}
	reg, _ := config.LoadSites()
	if reg.Sites[0].AutoSnapshot != config.AutoSnapshotOff {
		t.Errorf("site override = %q, want off", reg.Sites[0].AutoSnapshot)
	}

	res, _ = execDBAuto(nil)
	text := toolText(res)
	if !strings.Contains(text, `"covered": false`) || !strings.Contains(text, "shop_db") {
		t.Errorf("auto should report the opted-out site: %s", text)
	}
}

func TestExecDBAutoSet_selection(t *testing.T) {
	autoSnapshotEnv(t)

	res, rpcErr := execDBAutoSet(map[string]any{"enabled": true, "selection": "opt-in"})
	if rpcErr != nil {
		t.Fatalf("auto_set: %v", rpcErr)
	}
	text := toolText(res)
	if !strings.Contains(text, `"selection": "opt_in"`) {
		t.Fatalf("selection not reported: %s", text)
	}
	// Opt-in covers nothing until a site opts in, which the listing must show.
	if !strings.Contains(text, `"covered": false`) {
		t.Errorf("a site that says nothing should not be covered under opt-in: %s", text)
	}

	res, _ = execDBAutoSet(map[string]any{"selection": "maybe"})
	if !strings.Contains(toolText(res), "selection") {
		t.Errorf("expected a selection complaint: %s", toolText(res))
	}
}

func TestExecDBAutoSet_rejectsBadDuration(t *testing.T) {
	autoSnapshotEnv(t)
	res, _ := execDBAutoSet(map[string]any{"enabled": true, "every": "sometimes"})
	if !strings.Contains(toolText(res), "not a duration") {
		t.Errorf("expected a duration complaint: %s", toolText(res))
	}
	cfg, _ := config.LoadGlobal()
	if cfg.AutoSnapshot.Every != "" {
		t.Errorf("a rejected auto_set stored %q", cfg.AutoSnapshot.Every)
	}
}

func TestExecDBSnapshotKeep(t *testing.T) {
	autoSnapshotEnv(t)
	dir := filepath.Join(config.SnapshotsDir(), "mysql", "databases", "shop_db", "auto-1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(serviceops.Snapshot{
		Name: "auto-1", Created: time.Now().UTC(), Service: "mysql", Family: "mysql", Database: "shop_db", Auto: true,
	})
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o600); err != nil {
		t.Fatal(err)
	}

	args := map[string]any{"service": "mysql", "database": "shop_db", "name": "auto-1"}
	if _, rpcErr := execDBSnapshotKeep(args); rpcErr != nil {
		t.Fatalf("snapshot_keep: %v", rpcErr)
	}
	snaps, err := serviceops.ListSnapshots("mysql", "shop_db", false)
	if err != nil || len(snaps) != 1 || !snaps[0].Kept {
		t.Fatalf("snapshot not kept: %+v (%v)", snaps, err)
	}

	args["kept"] = false
	if _, rpcErr := execDBSnapshotKeep(args); rpcErr != nil {
		t.Fatalf("release: %v", rpcErr)
	}
	snaps, _ = serviceops.ListSnapshots("mysql", "shop_db", false)
	if snaps[0].Kept {
		t.Error("snapshot should be back under retention")
	}

	res, _ := execDBSnapshotKeep(map[string]any{"service": "mysql", "database": "shop_db"})
	if !strings.Contains(toolText(res), "name is required") {
		t.Errorf("expected a missing-name complaint: %s", toolText(res))
	}
}
