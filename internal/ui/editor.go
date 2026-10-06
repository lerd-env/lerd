package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/editor"
)

// handleOpenEditor opens a file at a line in the host's editor for dashboard
// links such as a query's caller path. It requires dashboard-control authority,
// and paths are confined to the user's home directory.
func handleOpenEditor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !hasHostActionAuthority(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Path string `json:"path"`
		Line int    `json:"line"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	path := filepath.Clean(req.Path)
	if !filepath.IsAbs(path) {
		http.Error(w, "path must be absolute", http.StatusBadRequest)
		return
	}
	home, _ := os.UserHomeDir()
	if home == "" || !strings.HasPrefix(path, home+string(os.PathSeparator)) {
		http.Error(w, "path outside home", http.StatusForbidden)
		return
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	// An editor only reachable by its URL is handed back for the dashboard to open.
	argv, url, err := editor.For(path, req.Line)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if url != "" {
		writeJSON(w, map[string]string{"url": url})
		return
	}
	if len(argv) == 0 {
		http.Error(w, "no editor found; set `editor` in ~/.config/lerd/config.yaml", http.StatusInternalServerError)
		return
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		http.Error(w, fmt.Sprintf("launching editor: %v", err), http.StatusInternalServerError)
		return
	}
	go func() { _ = cmd.Wait() }() // reap; the editor detaches
	w.WriteHeader(http.StatusNoContent)
}

// handleEditors lists the editors to choose from with the global choice (GET),
// or saves the global choice (POST {"id"}), a listed editor or a custom
// template, which a read reports as "custom" with the template beside it. Only
// the host itself may: the choice is a command the host runs on a later click.
func handleEditors(w http.ResponseWriter, r *http.Request) {
	if !isLocalControlRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost {
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if !editor.Valid(body.ID) {
			writeJSON(w, map[string]any{"ok": false, "error": fmt.Sprintf("unknown editor %q", body.ID)})
			return
		}
		cfg, err := config.LoadGlobal()
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		cfg.Editor = body.ID
		if err := config.SaveGlobal(cfg); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	global := ""
	if cfg, _ := config.LoadGlobal(); cfg != nil {
		global = strings.TrimSpace(cfg.Editor)
	}
	template := ""
	if _, ok := editor.Known(global); global != "" && !ok {
		global, template = "custom", global
	}
	type choice struct {
		editor.Editor
		Installed bool `json:"installed"`
	}
	list := make([]choice, 0, len(editor.Editors))
	for _, e := range editor.Editors {
		list = append(list, choice{e, e.Installed()})
	}
	writeJSON(w, map[string]any{"editors": list, "global": global, "template": template})
}

// openProjectInEditor opens a site's folder, or one of its worktrees, as a
// project in the chosen editor. Unlike file links it never falls back to an
// editor found by probing, so the action only exists once one is set.
func openProjectInEditor(site config.Site, dir string) error {
	choice := ""
	if cfg, _ := config.LoadGlobal(); cfg != nil {
		choice = strings.TrimSpace(cfg.Editor)
	}
	if choice == "" {
		return fmt.Errorf("no editor set for %s", site.Name)
	}
	argv := editor.DirCommand(dir)
	if len(argv) == 0 {
		return fmt.Errorf("%s cannot open a folder from here", choice)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching editor: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
