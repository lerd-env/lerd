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

func TestServicePresentation_ResolvesFromPreset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	pres := resolvePresentation("phpmyadmin", nil)
	if pres.Category != "admin" || pres.Icon != "database" {
		t.Errorf("phpmyadmin should present as admin/database, got %q/%q", pres.Category, pres.Icon)
	}
	if len(pres.AdminFor) != 2 || pres.AdminFor[0] != "mysql" || pres.AdminFor[1] != "mariadb" {
		t.Errorf("phpmyadmin should administer mysql and mariadb, got %v", pres.AdminFor)
	}
}

// A versioned family member (mariadb-11-8) carries no metadata of its own; it
// resolves through the preset it was installed from.
func TestServicePresentation_ResolvesVersionedFamilyMemberViaPreset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	custom := &config.CustomService{Name: "mariadb-11-8", Preset: "mariadb"}
	pres := resolvePresentation("mariadb-11-8", custom)
	if pres.Category != "databases" || pres.Icon != "database" {
		t.Errorf("mariadb-11-8 should inherit the mariadb preset's databases/database, got %q/%q", pres.Category, pres.Icon)
	}
}

// A service installed before these fields existed has no category in its stored
// YAML, but its preset does, so the live preset must win over the stale copy.
func TestServicePresentation_PrefersPresetOverStaleStoredYAML(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	stale := &config.CustomService{Name: "redisinsight", Preset: "redisinsight"}
	pres := resolvePresentation("redisinsight", stale)
	if pres.Category != "admin" || pres.Icon != "database" {
		t.Errorf("stale redisinsight should still present as admin/database, got %q/%q", pres.Category, pres.Icon)
	}
	if len(pres.AdminFor) != 2 || pres.AdminFor[1] != "valkey" {
		t.Errorf("redisinsight should administer valkey, got %v", pres.AdminFor)
	}
}

// A user-defined service is in no preset, so its own YAML is the only source.
func TestServicePresentation_FallsBackToUserDefinedYAML(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	own := &config.CustomService{Name: "my-thing", Category: "testing", Icon: "browserPlay"}
	pres := resolvePresentation("my-thing", own)
	if pres.Category != "testing" || pres.Icon != "browserPlay" {
		t.Errorf("user-defined service should use its own metadata, got %q/%q", pres.Category, pres.Icon)
	}
}

// A colour is the one part of the metadata that becomes CSS on the page, so
// anything but a plain hex has to be dropped before it leaves the daemon.
func TestServicePresentation_DropsAColourThatIsNotPlainHex(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	good := resolvePresentation("my-thing", &config.CustomService{Name: "my-thing", Color: "#00758F"})
	if good.Color != "#00758f" {
		t.Errorf("a hex colour should survive, got %q", good.Color)
	}
	bad := resolvePresentation("my-thing", &config.CustomService{Name: "my-thing", Color: "url(https://evil.test/x)"})
	if bad.Color != "" {
		t.Errorf("a non-hex colour should be dropped, got %q", bad.Color)
	}
}

// The preset list feeds the suggestion cards, so a preset's admin_for has to
// survive the hop from config.PresetMeta into PresetResponse.
func TestHandleServicePresets_CarriesDiscoveryMetadata(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	rec := httptest.NewRecorder()
	handleServicePresets(rec, httptest.NewRequest(http.MethodGet, "/api/services/presets", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []PresetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byName := map[string]PresetResponse{}
	for _, p := range got {
		byName[p.Name] = p
	}
	pma, ok := byName["phpmyadmin"]
	if !ok {
		t.Fatal("preset list missing phpmyadmin")
	}
	if pma.Category != "admin" || pma.Icon != "database" {
		t.Errorf("phpmyadmin should serialise as admin/database, got %q/%q", pma.Category, pma.Icon)
	}
	if len(pma.AdminFor) != 2 || pma.AdminFor[1] != "mariadb" {
		t.Errorf("phpmyadmin admin_for must reach the UI, got %v", pma.AdminFor)
	}
}

// The dashboard inlines these marks, so the endpoint must hand it the sanitized
// cached copy and nothing the browser has to fetch from the store itself.
func TestHandleServiceIcons_ServesTheCachedMarks(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	raw := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#c0ffee"><path d="M3 3h18v18H3z"/><script>bad()</script></svg>`
	if err := config.SaveStorePresetIcon("demo", []byte(raw)); err != nil {
		t.Fatalf("SaveStorePresetIcon: %v", err)
	}

	rec := httptest.NewRecorder()
	handleServiceIcons(rec, httptest.NewRequest(http.MethodGet, "/api/services/icons", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	svg, ok := got["demo"]
	if !ok {
		t.Fatalf("icons response missing demo: %v", got)
	}
	if !strings.Contains(svg, `d="M3 3h18v18H3z"`) {
		t.Errorf("served icon lost its drawing: %s", svg)
	}
	if strings.Contains(svg, "bad()") || strings.Contains(svg, "#c0ffee") {
		t.Errorf("served icon is not the sanitized copy: %s", svg)
	}
}

// With nothing cached the default stack still has the marks the binary ships,
// and the dashboard gets an object it can iterate rather than a null to guard.
func TestHandleServiceIcons_ServesTheShippedMarksWithAnEmptyCache(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	rec := httptest.NewRecorder()
	handleServiceIcons(rec, httptest.NewRequest(http.MethodGet, "/api/services/icons", nil))
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(got["mysql"], "<path") {
		t.Errorf("mysql should serve its shipped mark, got %q", got["mysql"])
	}
}

// suggest_for drives the suggestion card on another service's page, so it has
// to reach the preset list the dashboard reads.
func TestHandleServicePresets_CarriesSuggestFor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	presets := filepath.Join(dir, "lerd", "service-presets")
	if err := os.MkdirAll(presets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(presets, "spamassassin.yaml"), []byte("name: spamassassin\nimage: x\nsuggest_for:\n  - mailpit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handleServicePresets(rec, httptest.NewRequest(http.MethodGet, "/api/services/presets", nil))
	var got []PresetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, p := range got {
		if p.Name == "spamassassin" {
			if len(p.SuggestFor) != 1 || p.SuggestFor[0] != "mailpit" {
				t.Errorf("suggest_for = %v, want [mailpit]", p.SuggestFor)
			}
			return
		}
	}
	t.Fatal("spamassassin missing from the preset list")
}
