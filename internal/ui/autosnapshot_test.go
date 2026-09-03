package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// seedAutoSnapshotSite registers a site whose .env points at a lerd engine.
func seedAutoSnapshotSite(t *testing.T, name, database, mode string) config.Site {
	t.Helper()
	dir := t.TempDir()
	env := "DB_CONNECTION=mysql\nDB_HOST=lerd-mysql\nDB_DATABASE=" + database + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	return config.Site{Name: name, Domains: []string{name + ".test"}, Path: dir, PHPVersion: "8.4", AutoSnapshot: mode}
}

func TestHandleAutoSnapshot_readAndSave(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{
		seedAutoSnapshotSite(t, "shop", "shop_db", config.AutoSnapshotDefault),
		seedAutoSnapshotSite(t, "blog", "blog_db", config.AutoSnapshotOff),
	}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	rec := httptest.NewRecorder()
	handleAutoSnapshot(rec, httptest.NewRequest(http.MethodGet, "/api/auto-snapshot", nil))
	var got autoSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if !got.Enabled || got.Selection != config.AutoSnapshotOptIn {
		t.Errorf("a fresh install should ship on and opt-in, got enabled=%v selection=%q", got.Enabled, got.Selection)
	}
	for _, site := range got.Sites {
		if site.Covered {
			t.Errorf("%s is covered before anything opted in", site.Site)
		}
	}
	if len(got.Sites) != 2 {
		t.Fatalf("got %d sites, want both listed whether covered or not", len(got.Sites))
	}

	rec = httptest.NewRecorder()
	body := `{"enabled":true,"every":"6h","keep":5,"keep_for":"168h","selection":"opt-out"}`
	handleAutoSnapshot(rec, httptest.NewRequest(http.MethodPost, "/api/auto-snapshot", strings.NewReader(body)))
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode save: %v (%s)", err, rec.Body.String())
	}
	if !got.Enabled || got.Every != "6h0m0s" || got.Keep != 5 || got.KeepFor != "168h0m0s" {
		t.Fatalf("saved policy = %+v", got)
	}
	for _, site := range got.Sites {
		wantCovered := site.Site == "shop"
		if site.Covered != wantCovered {
			t.Errorf("%s covered = %v, want %v", site.Site, site.Covered, wantCovered)
		}
	}
}

// Opt-in covers nothing until a site says yes, which the site listing has to
// reflect or the dashboard would promise snapshots nobody takes.
func TestHandleAutoSnapshot_optInCoversNothingByDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{
		seedAutoSnapshotSite(t, "shop", "shop_db", config.AutoSnapshotDefault),
		seedAutoSnapshotSite(t, "blog", "blog_db", config.AutoSnapshotOn),
	}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	rec := httptest.NewRecorder()
	body := `{"enabled":true,"every":"24h","keep":7,"selection":"opt-in"}`
	handleAutoSnapshot(rec, httptest.NewRequest(http.MethodPost, "/api/auto-snapshot", strings.NewReader(body)))
	var got autoSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if got.Selection != config.AutoSnapshotOptIn {
		t.Fatalf("selection = %q, want opt_in", got.Selection)
	}
	for _, site := range got.Sites {
		wantCovered := site.Site == "blog"
		if site.Covered != wantCovered {
			t.Errorf("%s covered = %v, want %v under opt-in", site.Site, site.Covered, wantCovered)
		}
	}
}

func TestHandleAutoSnapshot_rejectsBadSelection(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	rec := httptest.NewRecorder()
	handleAutoSnapshot(rec, httptest.NewRequest(http.MethodPost, "/api/auto-snapshot", strings.NewReader(`{"enabled":true,"selection":"maybe","every":"6h"}`)))
	if !strings.Contains(rec.Body.String(), "selection") {
		t.Errorf("expected a selection complaint, got %s", rec.Body.String())
	}
	cfg, _ := config.LoadGlobal()
	if cfg.AutoSnapshot.Every != "" {
		t.Errorf("a rejected save stored %q", cfg.AutoSnapshot.Every)
	}
}

func TestHandleAutoSnapshot_rejectsBadSchedule(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	rec := httptest.NewRecorder()
	handleAutoSnapshot(rec, httptest.NewRequest(http.MethodPost, "/api/auto-snapshot", strings.NewReader(`{"enabled":true,"every":"sometimes"}`)))
	if !strings.Contains(rec.Body.String(), "duration") {
		t.Errorf("expected a duration complaint, got %s", rec.Body.String())
	}
	cfg, _ := config.LoadGlobal()
	if cfg.AutoSnapshot.Every != "" {
		t.Errorf("a rejected save stored %q", cfg.AutoSnapshot.Every)
	}
}

func TestHandleAutoSnapshotSite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{
		seedAutoSnapshotSite(t, "shop", "shop_db", config.AutoSnapshotDefault),
	}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auto-snapshot/site", strings.NewReader(`{"site":"shop","mode":"on"}`))
	handleAutoSnapshotSite(rec, req)
	var got autoSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if len(got.Sites) != 1 || got.Sites[0].Mode != config.AutoSnapshotOn || !got.Sites[0].Covered {
		t.Fatalf("site status = %+v", got.Sites)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/auto-snapshot/site", strings.NewReader(`{"site":"shop","mode":"whenever"}`))
	handleAutoSnapshotSite(rec, req)
	if !strings.Contains(rec.Body.String(), "mode") {
		t.Errorf("expected an unknown-mode complaint, got %s", rec.Body.String())
	}
}
