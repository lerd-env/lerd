package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func isolateThemesDir(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := config.ThemesDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestHandleThemesListsAndReportsBadFiles(t *testing.T) {
	dir := isolateThemesDir(t)
	if err := os.WriteFile(filepath.Join(dir, "ocean.yaml"), []byte("name: Ocean\naccent: \"#3b7ea1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("name: Broken\naccent: nope\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleThemes(rec, httptest.NewRequest(http.MethodGet, "/api/themes", nil))

	var got struct {
		Themes []config.UITheme      `json:"themes"`
		Errors []config.UIThemeError `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if len(got.Themes) != 1 || got.Themes[0].ID != "ocean" {
		t.Fatalf("themes = %+v", got.Themes)
	}
	if len(got.Errors) != 1 || got.Errors[0].File != "broken.yaml" {
		t.Fatalf("errors = %+v", got.Errors)
	}
}

func TestHandleThemesImportWritesFile(t *testing.T) {
	dir := isolateThemesDir(t)

	body, _ := json.Marshal(map[string]string{"id": "ocean", "content": "name: Ocean\naccent: \"#3b7ea1\"\n"})
	rec := httptest.NewRecorder()
	handleThemes(rec, httptest.NewRequest(http.MethodPost, "/api/themes", bytes.NewReader(body)))

	if !strings.Contains(rec.Body.String(), "\"ok\":true") {
		t.Fatalf("expected ok=true, got %s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "ocean.yaml")); err != nil {
		t.Fatalf("theme not written: %v", err)
	}
}

func TestHandleThemesImportRejectsInvalid(t *testing.T) {
	isolateThemesDir(t)

	body, _ := json.Marshal(map[string]string{"id": "ocean", "content": "name: Ocean\naccent: url(evil)\n"})
	rec := httptest.NewRecorder()
	handleThemes(rec, httptest.NewRequest(http.MethodPost, "/api/themes", bytes.NewReader(body)))

	if !strings.Contains(rec.Body.String(), "\"ok\":false") {
		t.Fatalf("expected ok=false, got %s", rec.Body.String())
	}
}

func TestHandleThemeItemDeletes(t *testing.T) {
	dir := isolateThemesDir(t)
	path := filepath.Join(dir, "ocean.yaml")
	if err := os.WriteFile(path, []byte("name: Ocean\naccent: \"#3b7ea1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleThemeItem(rec, httptest.NewRequest(http.MethodDelete, "/api/themes/ocean", nil))

	if !strings.Contains(rec.Body.String(), "\"ok\":true") {
		t.Fatalf("expected ok=true, got %s", rec.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("theme still on disk: %v", err)
	}
}

func TestHandleThemeItemRefusesTraversal(t *testing.T) {
	isolateThemesDir(t)

	rec := httptest.NewRecorder()
	handleThemeItem(rec, httptest.NewRequest(http.MethodDelete, "/api/themes/..%2f..%2fconfig", nil))

	if !strings.Contains(rec.Body.String(), "\"ok\":false") {
		t.Fatalf("expected ok=false, got %s", rec.Body.String())
	}
}

func TestManifestColorAcceptsOnlyHex(t *testing.T) {
	if got := manifestColor("#3B7EA1", "#ff2d20"); got != "#3b7ea1" {
		t.Errorf("manifestColor(hex) = %q", got)
	}
	for _, bad := range []string{"", "rebeccapurple", `red"},"name":"evil`, "url(x)"} {
		if got := manifestColor(bad, "#ff2d20"); got != "#ff2d20" {
			t.Errorf("manifestColor(%q) = %q, want the fallback", bad, got)
		}
	}
}
