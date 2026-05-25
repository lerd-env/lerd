package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// validServiceNameRe matches the same shape as config.validServiceName
// (which is unexported). Used to reject path traversal / unsafe service
// names before any filesystem touch.
var validServiceNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// serviceEnvResponse is the shape returned by GET /api/services/{name}/env.
// Both maps are present (possibly empty) so the frontend can render a stable
// two-section editor without null-handling.
type serviceEnvResponse struct {
	Name        string            `json:"name"`
	Environment map[string]string `json:"environment"` // container Environment= lines (quadlet)
	EnvVars     []string          `json:"env_vars"`    // Laravel-side .env hints (read-only here)
	Source      string            `json:"source"`      // "preset" (built-in default) or "custom" (user override on disk)
}

// serviceEnvUpdate is the PUT body. Only `environment` is mutable — env_vars
// (the Laravel .env hints) come from the preset and apply at link time, not
// at runtime, so editing them at the service level wouldn't take effect.
type serviceEnvUpdate struct {
	Environment map[string]string `json:"environment"`
}

// handleServiceEnv serves and writes the container `Environment=` block for a
// lerd service. GET returns the current merged value (preset defaults + any
// user override file under ~/.config/lerd/services/<name>.yaml); PUT writes
// a user override and regenerates the quadlet so the next restart picks it up.
//
//	GET /api/services/{name}/env
//	PUT /api/services/{name}/env   body: {"environment": {"KEY": "VALUE", ...}}
func handleServiceEnv(w http.ResponseWriter, r *http.Request) {
	// Path: /api/services/{name}/env
	rest := strings.TrimPrefix(r.URL.Path, "/api/services/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "env" {
		http.NotFound(w, r)
		return
	}
	name := parts[0]
	if !validServiceNameRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		svc, source, err := loadServiceForEnv(name)
		if err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, serviceEnvResponse{
			Name:        name,
			Environment: svc.Environment,
			EnvVars:     svc.EnvVars,
			Source:      source,
		})
	case http.MethodPut:
		// Loopback-only — service env can contain credentials (DB passwords,
		// API keys). Same gate as /api/sites/{d}/env PUT.
		if !isLoopbackRequest(r) {
			http.Error(w, "forbidden: service env editing is loopback-only", http.StatusForbidden)
			return
		}
		var body serviceEnvUpdate
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Re-validate key/value content so a malicious payload can't inject
		// extra systemd directives into the rendered quadlet — SaveCustomService
		// already does this but we want a clean 400 instead of an unhelpful
		// "invalid environment value" wrap.
		for k, v := range body.Environment {
			if k == "" {
				http.Error(w, "empty environment key not allowed", http.StatusBadRequest)
				return
			}
			if strings.ContainsAny(v, "\n\r\x00") {
				http.Error(w, fmt.Sprintf("invalid value for %q: must not contain newline or NUL", k), http.StatusBadRequest)
				return
			}
		}

		svc, _, err := loadServiceForEnv(name)
		if err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		svc.Environment = body.Environment
		if err := config.SaveCustomService(svc); err != nil {
			writeJSON(w, map[string]any{"error": "saving service: " + err.Error()})
			return
		}
		// Re-render the quadlet so the new Environment= lines are picked up
		// on the next start. The caller is expected to restart the unit
		// (the UI does this via the existing /restart action).
		if err := ensureCustomServiceQuadletByName(name); err != nil {
			writeJSON(w, map[string]any{"error": "regenerating quadlet: " + err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// loadServiceForEnv returns the effective CustomService for name and a hint
// of where it came from: "custom" when an override exists in
// ~/.config/lerd/services/<name>.yaml, otherwise "preset" (using the
// embedded YAML's defaults).
func loadServiceForEnv(name string) (*config.CustomService, string, error) {
	if svc, err := config.LoadCustomService(name); err == nil {
		// Existing user override on disk.
		return svc, "custom", nil
	}
	// Fall back to the embedded preset; convert to a CustomService base
	// so the UI sees the preset's defaults as editable.
	p, err := config.LoadPreset(name)
	if err != nil {
		return nil, "", fmt.Errorf("unknown service %q", name)
	}
	svc := p.CustomService
	if svc.Name == "" {
		svc.Name = name
	}
	if svc.Environment == nil {
		svc.Environment = map[string]string{}
	}
	return &svc, "preset", nil
}

// ensureCustomServiceQuadletByName looks up the (just-saved) service config
// and re-renders its podman quadlet. Thin wrapper around the existing
// ensureCustomServiceQuadlet helper in server.go so the env-edit path
// stays consistent with the install/reinstall paths.
func ensureCustomServiceQuadletByName(name string) error {
	svc, err := config.LoadCustomService(name)
	if err != nil {
		return err
	}
	return ensureCustomServiceQuadlet(svc)
}
