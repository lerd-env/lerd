package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/geodro/lerd/internal/push"
)

var (
	toolUpdatesMu   sync.Mutex
	toolUpdatesPrev map[string]bool
)

// pendingTool is a managed host tool that has just fallen behind its pin.
type pendingTool struct {
	name    string
	version string
}

// notifyOnToolUpdates dispatches one notification per managed host tool whose
// pin has moved ahead of what is installed, so a republished Composer is
// announced the same way a service or a PHP base image is.
func notifyOnToolUpdates(statusJSON []byte) {
	for _, tool := range toolUpdateTransitions(statusJSON) {
		dispatchNotification(notificationForToolUpdate(tool.name, tool.version))
	}
}

// toolUpdateTransitions returns the tools whose update_available flag flipped
// false → true against the previous status snapshot, and records the new
// state. The first snapshot of a process seeds that state and reports nothing,
// so a machine that is already behind does not notify on startup.
func toolUpdateTransitions(statusJSON []byte) []pendingTool {
	if len(statusJSON) == 0 {
		return nil
	}
	var snapshot struct {
		Tools []struct {
			Name            string `json:"name"`
			Pinned          string `json:"pinned"`
			Present         bool   `json:"present"`
			UpdateAvailable bool   `json:"update_available"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(statusJSON, &snapshot); err != nil {
		fmt.Printf("[notify-update] status decode: %v\n", err)
		return nil
	}
	cur := make(map[string]bool, len(snapshot.Tools))
	pinned := make(map[string]string, len(snapshot.Tools))
	for _, t := range snapshot.Tools {
		// A tool that is absent shows its pin as the version an install would
		// bring, which is not something the user is behind on.
		cur[t.Name] = t.UpdateAvailable && t.Present
		pinned[t.Name] = t.Pinned
	}

	toolUpdatesMu.Lock()
	prev := toolUpdatesPrev
	toolUpdatesPrev = cur
	toolUpdatesMu.Unlock()

	if prev == nil {
		return nil
	}
	var out []pendingTool
	for _, name := range newUpdatesAvailable(prev, cur) {
		out = append(out, pendingTool{name: name, version: pinned[name]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func notificationForToolUpdate(name, version string) push.Notification {
	body := "Version " + version + " is pinned. Apply it from the Tools card."
	if version == "" {
		body = "A newer version is pinned. Apply it from the Tools card."
	}
	return push.Notification{
		Kind:     "update_available",
		TitleKey: "notify_update_title",
		Title:    "Update available: " + name,
		BodyKey:  "notify_tool_update_body",
		Body:     body,
		Params:   map[string]string{"service": name, "version": version},
		Tag:      "lerd-update-tool-" + name,
		URL:      "#system/tools",
		Data:     map[string]string{"service": name, "version": version},
		Urgency:  "low",
		TTL:      3600,
	}
}
