package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// handleOpenFolder opens a directory in the host's file manager (xdg-open on
// Linux, open on macOS). Loopback-only and confined to the user's home, the
// same guards as handleOpenEditor. Backs the clickable path on a site's header.
func handleOpenFolder(w http.ResponseWriter, r *http.Request) {
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
	// Resolve symlinks before the home check so a symlink under home that points
	// outside home can't slip a foreign path past the prefix guard. Home is
	// resolved the same way in case it is itself a symlink.
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		http.Error(w, "folder not found", http.StatusNotFound)
		return
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		if h, err := filepath.EvalSymlinks(home); err == nil {
			home = h
		}
	}
	if home == "" || (path != home && !strings.HasPrefix(path, home+string(os.PathSeparator))) {
		http.Error(w, "path outside home", http.StatusForbidden)
		return
	}
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		http.Error(w, "folder not found", http.StatusNotFound)
		return
	}

	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	bin, err := exec.LookPath(opener)
	if err != nil {
		http.Error(w, "no file manager opener found", http.StatusInternalServerError)
		return
	}
	cmd := exec.Command(bin, path)
	if err := cmd.Start(); err != nil {
		http.Error(w, fmt.Sprintf("launching file manager: %v", err), http.StatusInternalServerError)
		return
	}
	go func() { _ = cmd.Wait() }() // reap; the file manager detaches
	w.WriteHeader(http.StatusNoContent)
}
