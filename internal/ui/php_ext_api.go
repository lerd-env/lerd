package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// validExtName matches the same shape `lerd php:ext add` accepts.
// Letters, digits, hyphens, underscores — no shell metacharacters.
var validExtName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// phpExtItem is the shape returned by listPhpExtensions for each row.
type phpExtItem struct {
	Name    string   `json:"name"`
	ApkDeps []string `json:"apk_deps,omitempty"`
}

// addPhpExtRequest is the JSON body POSTed when adding an extension.
type addPhpExtRequest struct {
	Extension string   `json:"extension"`
	ApkDeps   []string `json:"apk_deps,omitempty"`
}

// phpExtResponse is the shared shape for add/remove responses.
type phpExtResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// handlePhpExtensionsList responds with the configured custom extensions for a
// PHP version. Path: GET /api/php-versions/{version}/extensions
func handlePhpExtensionsList(w http.ResponseWriter, r *http.Request) {
	version := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/php-versions/"), "/extensions")
	if !validVersion.MatchString(version) {
		http.NotFound(w, r)
		return
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		writeJSON(w, phpExtResponse{Error: err.Error()})
		return
	}
	exts := cfg.GetExtensions(version)
	items := make([]phpExtItem, 0, len(exts))
	for _, name := range exts {
		items = append(items, phpExtItem{Name: name, ApkDeps: cfg.GetExtApkDeps(name)})
	}
	writeJSON(w, map[string]any{"version": version, "extensions": items})
}

// handlePhpExtensionAdd installs a custom extension. Synchronous: blocks until
// the FPM image is rebuilt and verified, which takes 1–3 minutes. The Svelte
// frontend keeps a spinner up while the request is in flight.
// Path: POST /api/php-versions/{version}/extensions
func handlePhpExtensionAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	version := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/php-versions/"), "/extensions")
	if !validVersion.MatchString(version) {
		http.NotFound(w, r)
		return
	}

	var req addPhpExtRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, phpExtResponse{Error: "invalid JSON body: " + err.Error()})
		return
	}
	ext := strings.TrimSpace(req.Extension)
	if !validExtName.MatchString(ext) {
		writeJSON(w, phpExtResponse{Error: "invalid extension name (use letters, digits, hyphens, underscores)"})
		return
	}

	// Validate apk dep names with the same regex the CLI uses so a malicious
	// value can't break out of the generated `apk add` shell command.
	deps, err := podman.ParseApkDeps(strings.Join(req.ApkDeps, " "))
	if err != nil {
		writeJSON(w, phpExtResponse{Error: err.Error()})
		return
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		writeJSON(w, phpExtResponse{Error: err.Error()})
		return
	}

	cfg.AddExtension(version, ext)
	if len(deps) > 0 {
		cfg.SetExtApkDeps(ext, deps)
	}
	if err := config.SaveGlobal(cfg); err != nil {
		writeJSON(w, phpExtResponse{Error: "saving config: " + err.Error()})
		return
	}

	if err := podman.RebuildFPMImage(version, false); err != nil {
		// Roll back the config write so the user can retry cleanly.
		cfg.RemoveExtension(version, ext)
		_ = config.SaveGlobal(cfg)
		writeJSON(w, phpExtResponse{Error: "rebuild failed: " + err.Error()})
		return
	}

	if err := podman.VerifyExtensionLoaded(version, ext); err != nil {
		cfg.RemoveExtension(version, ext)
		_ = config.SaveGlobal(cfg)
		writeJSON(w, phpExtResponse{Error: fmt.Sprintf("extension did not load after rebuild (config reverted): %v", err)})
		return
	}

	short := strings.ReplaceAll(version, ".", "")
	if err := podman.RestartUnit("lerd-php" + short + "-fpm"); err != nil {
		// Extension is installed; just couldn't bounce the unit. Report
		// success with a soft warning so the caller knows to restart.
		writeJSON(w, phpExtResponse{OK: true, Error: "installed, but FPM restart failed: " + err.Error()})
		return
	}
	writeJSON(w, phpExtResponse{OK: true})
}

// handlePhpExtensionRemove uninstalls a custom extension. Same synchronous
// rebuild flow as Add. Path: DELETE /api/php-versions/{version}/extensions/{ext}
func handlePhpExtensionRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	// Path: /api/php-versions/{version}/extensions/{ext}
	tail := strings.TrimPrefix(r.URL.Path, "/api/php-versions/")
	parts := strings.SplitN(tail, "/", 3)
	if len(parts) != 3 || parts[1] != "extensions" {
		http.NotFound(w, r)
		return
	}
	version, ext := parts[0], parts[2]
	if !validVersion.MatchString(version) || !validExtName.MatchString(ext) {
		http.NotFound(w, r)
		return
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		writeJSON(w, phpExtResponse{Error: err.Error()})
		return
	}
	cfg.RemoveExtension(version, ext)
	if err := config.SaveGlobal(cfg); err != nil {
		writeJSON(w, phpExtResponse{Error: "saving config: " + err.Error()})
		return
	}
	if err := podman.RebuildFPMImage(version, false); err != nil {
		writeJSON(w, phpExtResponse{Error: "rebuild failed: " + err.Error()})
		return
	}
	short := strings.ReplaceAll(version, ".", "")
	if err := podman.RestartUnit("lerd-php" + short + "-fpm"); err != nil {
		writeJSON(w, phpExtResponse{OK: true, Error: "removed, but FPM restart failed: " + err.Error()})
		return
	}
	writeJSON(w, phpExtResponse{OK: true})
}
