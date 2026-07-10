package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestServicePresentation_ResolvesFromPreset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	category, icon, adminFor := servicePresentation("phpmyadmin", nil)
	if category != "admin" || icon != "database" {
		t.Errorf("phpmyadmin should present as admin/database, got %q/%q", category, icon)
	}
	if len(adminFor) != 2 || adminFor[0] != "mysql" || adminFor[1] != "mariadb" {
		t.Errorf("phpmyadmin should administer mysql and mariadb, got %v", adminFor)
	}
}

// A versioned family member (mariadb-11-8) carries no metadata of its own; it
// resolves through the preset it was installed from.
func TestServicePresentation_ResolvesVersionedFamilyMemberViaPreset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	custom := &config.CustomService{Name: "mariadb-11-8", Preset: "mariadb"}
	category, icon, _ := servicePresentation("mariadb-11-8", custom)
	if category != "databases" || icon != "database" {
		t.Errorf("mariadb-11-8 should inherit the mariadb preset's databases/database, got %q/%q", category, icon)
	}
}

// A service installed before these fields existed has no category in its stored
// YAML, but its preset does, so the live preset must win over the stale copy.
func TestServicePresentation_PrefersPresetOverStaleStoredYAML(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	stale := &config.CustomService{Name: "redisinsight", Preset: "redisinsight"}
	category, icon, adminFor := servicePresentation("redisinsight", stale)
	if category != "admin" || icon != "database" {
		t.Errorf("stale redisinsight should still present as admin/database, got %q/%q", category, icon)
	}
	if len(adminFor) != 2 || adminFor[1] != "valkey" {
		t.Errorf("redisinsight should administer valkey, got %v", adminFor)
	}
}

// A user-defined service is in no preset, so its own YAML is the only source.
func TestServicePresentation_FallsBackToUserDefinedYAML(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	own := &config.CustomService{Name: "my-thing", Category: "testing", Icon: "browserPlay"}
	category, icon, _ := servicePresentation("my-thing", own)
	if category != "testing" || icon != "browserPlay" {
		t.Errorf("user-defined service should use its own metadata, got %q/%q", category, icon)
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
