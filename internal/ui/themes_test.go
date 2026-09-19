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

func TestHandleSettingsThemePersists(t *testing.T) {
	isolateThemesDir(t)

	body, _ := json.Marshal(map[string]string{"theme": "nord"})
	rec := httptest.NewRecorder()
	handleSettingsTheme(rec, httptest.NewRequest(http.MethodPost, "/api/settings/theme", bytes.NewReader(body)))

	if !strings.Contains(rec.Body.String(), "\"ok\":true") {
		t.Fatalf("expected ok=true, got %s", rec.Body.String())
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UI.Theme != "nord" {
		t.Errorf("cfg.UI.Theme = %q, want nord", cfg.UI.Theme)
	}
}

func TestHandleSettingsThemeAcceptsTheDefaultBack(t *testing.T) {
	isolateThemesDir(t)

	for _, want := range []string{"nord", ""} {
		body, _ := json.Marshal(map[string]string{"theme": want})
		rec := httptest.NewRecorder()
		handleSettingsTheme(rec, httptest.NewRequest(http.MethodPost, "/api/settings/theme", bytes.NewReader(body)))
		cfg, _ := config.LoadGlobal()
		if cfg.UI.Theme != want {
			t.Fatalf("cfg.UI.Theme = %q, want %q", cfg.UI.Theme, want)
		}
	}
}

func TestHandleSettingsThemeRefusesJunk(t *testing.T) {
	isolateThemesDir(t)

	body, _ := json.Marshal(map[string]string{"theme": "../../etc/passwd"})
	rec := httptest.NewRecorder()
	handleSettingsTheme(rec, httptest.NewRequest(http.MethodPost, "/api/settings/theme", bytes.NewReader(body)))

	if !strings.Contains(rec.Body.String(), "\"ok\":false") {
		t.Fatalf("expected ok=false, got %s", rec.Body.String())
	}
	cfg, _ := config.LoadGlobal()
	if cfg.UI.Theme != "" {
		t.Errorf("cfg.UI.Theme = %q, want empty", cfg.UI.Theme)
	}
}

func TestAssembleSnapshot_IncludesThemeField(t *testing.T) {
	frame := assembleSnapshot(nil, nil, nil, nil, nil, nil, nil, nil, []byte(`"nord"`), []string{"theme"})
	var decoded struct {
		Type  string `json:"type"`
		Theme string `json:"theme"`
	}
	if err := json.Unmarshal(frame, &decoded); err != nil {
		t.Fatalf("decode %s: %v", frame, err)
	}
	if decoded.Type != "theme" || decoded.Theme != "nord" {
		t.Errorf("frame = %s", frame)
	}
}

func TestBroadcastThemeReachesEveryPeer(t *testing.T) {
	b := &wsBroker{peers: make(map[chan wsMessage]struct{})}
	one, two := b.add(), b.add()

	b.broadcastTheme("gruvbox")

	for i, ch := range []chan wsMessage{one, two} {
		msg := <-ch
		if len(msg.Kinds) != 1 || msg.Kinds[0] != "theme" {
			t.Fatalf("peer %d kinds = %v", i, msg.Kinds)
		}
		if string(msg.Theme) != `"gruvbox"` {
			t.Errorf("peer %d theme = %s", i, msg.Theme)
		}
	}
}

// withOmarchyTheme lays out the state directory Omarchy keeps its active theme
// in, so the handler is exercised against a real tree.
func installOmarchyTheme(t *testing.T, name, colors string) {
	t.Helper()
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	dir := filepath.Join(state, "omarchy", "current", "theme")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "colors.toml"), []byte(colors), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "omarchy", "current", "theme.name"), []byte(name), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestHandleThemesOffersTheDesktopTheme(t *testing.T) {
	isolateThemesDir(t)
	installOmarchyTheme(t, "tokyo-night", "mode = \"dark\"\naccent = \"#7aa2f7\"\nbackground = \"#1a1b26\"\n")

	rec := httptest.NewRecorder()
	handleThemes(rec, httptest.NewRequest(http.MethodGet, "/api/themes", nil))

	var got struct {
		Themes []config.UITheme `json:"themes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	var found *config.UITheme
	for i := range got.Themes {
		if got.Themes[i].ID == config.OmarchyThemeID {
			found = &got.Themes[i]
		}
	}
	if found == nil {
		t.Fatalf("themes = %+v, want the desktop theme among them", got.Themes)
	}
	if found.Accent != "#7aa2f7" {
		t.Errorf("Accent = %q, want the desktop accent", found.Accent)
	}
	if found.Name != "Omarchy (tokyo-night)" {
		t.Errorf("Name = %q, want the desktop theme named", found.Name)
	}
}

func TestHandleThemesWithoutOmarchyOffersNothingExtra(t *testing.T) {
	isolateThemesDir(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	rec := httptest.NewRecorder()
	handleThemes(rec, httptest.NewRequest(http.MethodGet, "/api/themes", nil))

	var got struct {
		Themes []config.UITheme `json:"themes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	for _, th := range got.Themes {
		if th.ID == config.OmarchyThemeID {
			t.Errorf("themes carry %q where Omarchy is not installed", th.ID)
		}
	}
}

// A theme file named omarchy.yaml must not quietly replace the desktop entry,
// or picking the desktop theme would silently get someone else's colours.
func TestHandleThemesKeepsTheDesktopEntryOverAFileOfTheSameName(t *testing.T) {
	dir := isolateThemesDir(t)
	if err := os.WriteFile(filepath.Join(dir, "omarchy.yaml"), []byte("name: Impostor\naccent: \"#ff0000\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	installOmarchyTheme(t, "nord", "mode = \"dark\"\naccent = \"#81a1c1\"\n")

	rec := httptest.NewRecorder()
	handleThemes(rec, httptest.NewRequest(http.MethodGet, "/api/themes", nil))

	var got struct {
		Themes []config.UITheme `json:"themes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	seen := 0
	for _, th := range got.Themes {
		if th.ID != config.OmarchyThemeID {
			continue
		}
		seen++
		if th.Accent != "#81a1c1" {
			t.Errorf("Accent = %q, want the desktop accent rather than the file's", th.Accent)
		}
	}
	if seen != 1 {
		t.Errorf("found %d entries for %q, want exactly one", seen, config.OmarchyThemeID)
	}
}
