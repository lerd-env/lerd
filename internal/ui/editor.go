package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// handleOpenEditor opens a file at a line in the user's editor, for the
// "open in editor" links in the dashboard (e.g. a query's caller path).
// Loopback-only: it execs a process on the host, so only a local browser
// session may trigger it. Paths are confined to the user's home directory.
func handleOpenEditor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackRequest(r) {
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

	argv := editorCommand(path, req.Line)
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

// editorCommand resolves the argv to open file:line. A configured `editor`
// template wins (with {file}/{line} substitution, or the file appended when it
// has neither placeholder); otherwise the first GUI editor found on PATH is
// used, falling back to the platform opener.
func editorCommand(file string, line int) []string {
	if cfg, _ := config.LoadGlobal(); cfg != nil && strings.TrimSpace(cfg.Editor) != "" {
		tmpl := strings.TrimSpace(cfg.Editor)
		if strings.Contains(tmpl, "{file}") || strings.Contains(tmpl, "{line}") {
			tmpl = strings.ReplaceAll(tmpl, "{file}", file)
			tmpl = strings.ReplaceAll(tmpl, "{line}", strconv.Itoa(line))
			return strings.Fields(tmpl)
		}
		return append(strings.Fields(tmpl), file)
	}

	loc := fmt.Sprintf("%s:%d", file, line)
	ls := strconv.Itoa(line)
	for _, c := range []struct {
		bin  string
		args []string
	}{
		{"code", []string{"-g", loc}},
		{"cursor", []string{"-g", loc}},
		{"codium", []string{"-g", loc}},
		{"windsurf", []string{"-g", loc}},
		{"subl", []string{loc}},
		{"zed", []string{loc}},
		{"phpstorm", []string{"--line", ls, file}},
		{"idea", []string{"--line", ls, file}},
	} {
		if p, err := exec.LookPath(c.bin); err == nil {
			return append([]string{p}, c.args...)
		}
	}
	// Last resort: hand the file to the platform opener (uses the default app).
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	if p, err := exec.LookPath(opener); err == nil {
		return []string{p, file}
	}
	return nil
}
