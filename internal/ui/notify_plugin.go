package ui

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/geodro/lerd/internal/plugins"
	"github.com/geodro/lerd/internal/push"
)

// pluginFailureNotification tells the user a plugin failed and takes them to
// the plugin manager, where its row offers to report it to the author.
func pluginFailureNotification(f plugins.Failure) push.Notification {
	first, _, _ := strings.Cut(f.Error, "\n")
	first = truncate(first, 200)
	return push.Notification{
		Kind:     "plugin_failed",
		TitleKey: "notify_plugin_failed_title",
		Title:    "Plugin " + f.Plugin + " failed",
		BodyKey:  "notify_plugin_failed_body",
		Body:     f.What + ": " + first,
		Params:   map[string]string{"plugin": f.Plugin, "what": f.What, "message": first},
		Tag:      "lerd-plugin-failed-" + f.Plugin,
		URL:      "#system/plugins",
		Data:     map[string]string{"plugin": f.Plugin},
		Urgency:  "normal",
		TTL:      3600,
	}
}

// notifyPluginFailures has failures in the dashboard's own process, from
// plugin endpoints and site commands, notify straight away.
func notifyPluginFailures() {
	plugins.SetFailureNotifier(func(f plugins.Failure) { dispatchNotification(pluginFailureNotification(f)) })
}

// handleInternalPluginFailed raises the notification for a failure another
// lerd process recorded, such as a hook the watcher ran.
func handleInternalPluginFailed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Plugin string `json:"plugin"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if f, ok := plugins.LastFailure(req.Plugin); ok {
		dispatchNotification(pluginFailureNotification(f))
	}
	w.WriteHeader(http.StatusNoContent)
}
