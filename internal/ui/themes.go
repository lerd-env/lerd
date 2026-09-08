package ui

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// handleThemes lists the dashboard themes the user has written into
// ~/.config/lerd/themes, and imports one. The built-in themes live in the
// dashboard itself, so this answers with the user's own files alone, plus the
// ones that could not be parsed so the picker can say why.
func handleThemes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		themes, errs := config.UIThemes()
		if themes == nil {
			themes = []config.UITheme{}
		}
		if errs == nil {
			errs = []config.UIThemeError{}
		}
		writeJSON(w, map[string]any{"themes": themes, "errors": errs})
	case http.MethodPost:
		var body struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
			return
		}
		if err := config.SaveUITheme(body.ID, []byte(body.Content)); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": body.ID})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleThemeItem removes one imported theme.
func handleThemeItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/themes/")
	if err := config.DeleteUITheme(id); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// manifestColor keeps a colour the dashboard put on the manifest URL to a plain
// hex, falling back to the brand default. The value lands inside a JSON literal
// the browser reads as the installed app's chrome, so nothing but a colour may
// reach it.
func manifestColor(v, fallback string) string {
	if c := config.NormalizeBrandColor(v); c != "" {
		return c
	}
	return fallback
}

// handleSettingsTheme persists which theme the dashboard is on. It lives in the
// global config rather than in the browser so every device that opens the
// dashboard agrees on what lerd looks like; the light/dark mode stays local,
// since that follows the room rather than the install.
func handleSettingsTheme(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Theme string `json:"theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	id := strings.TrimSpace(body.Theme)
	if id != "" && !validThemeName.MatchString(id) {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid theme name"})
		return
	}
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "loading config"})
		return
	}
	cfg.UI.Theme = id
	if err := config.SaveGlobal(cfg); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	broker.broadcastTheme(id)
	writeJSON(w, map[string]any{"ok": true, "theme": id})
}

// validThemeName is the shape a theme id may take, whether it names a built-in
// or a file. The value is written to the config and handed back to every client,
// so it is kept to the same slug a file could be called.
var validThemeName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
