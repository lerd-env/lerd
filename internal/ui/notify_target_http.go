package ui

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/desktopnotify"
)

type notifyTargetResponse struct {
	Target          string          `json:"target"`
	NativeSupported bool            `json:"native_supported"`
	AppInstalled    bool            `json:"app_installed"`
	Kinds           map[string]bool `json:"kinds"`
}

func newNotifyTargetResponse(cfg *config.GlobalConfig) notifyTargetResponse {
	r := notifyTargetResponse{
		Target:          config.NotifyTargetBrowser,
		NativeSupported: desktopnotify.Supported(),
		AppInstalled:    desktopnotify.AppInstalled(),
	}
	if cfg != nil {
		r.Target = cfg.NotificationTarget()
		r.Kinds = cfg.EffectiveNativeKinds()
	}
	return r
}

// handleNotifyTarget reads or sets the notification delivery sink. The response
// carries native_supported so the UI can gate the native option on hosts (or
// platforms) without a notification daemon, and the resolved native kind prefs.
func handleNotifyTarget(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, _ := config.LoadGlobal()
		writeJSON(w, newNotifyTargetResponse(cfg))
	case http.MethodPost:
		if !isLoopbackRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var req struct {
			Target string `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if req.Target != config.NotifyTargetBrowser && req.Target != config.NotifyTargetNative {
			http.Error(w, "target must be browser or native", http.StatusBadRequest)
			return
		}
		cfg, err := config.LoadGlobal()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cfg.SetNotificationTarget(req.Target)
		if err := config.SaveGlobal(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, newNotifyTargetResponse(cfg))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleNotifyKinds sets one native category on or off. Loopback-only; the
// browser sink's per-category prefs stay per-device in the page.
func handleNotifyKinds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Kind    string `json:"kind"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if !slices.Contains(config.NotifyKinds, req.Kind) {
		http.Error(w, "unknown kind", http.StatusBadRequest)
		return
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.SetNativeKind(req.Kind, req.Enabled)
	if err := config.SaveGlobal(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, newNotifyTargetResponse(cfg))
}
