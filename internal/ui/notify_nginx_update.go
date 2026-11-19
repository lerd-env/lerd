package ui

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/geodro/lerd/internal/push"
)

var (
	nginxUpdateMu   sync.Mutex
	nginxUpdatePrev *bool
)

// notifyOnNginxUpdate announces a republished nginx image the same way a
// service or PHP base update is announced.
func notifyOnNginxUpdate(statusJSON []byte) {
	if nginxUpdateTransition(statusJSON) {
		dispatchNotification(notificationForNginxUpdate())
	}
}

// nginxUpdateTransition reports whether the nginx update flag flipped false →
// true since the previous snapshot. The first snapshot only seeds the state, so
// an update already pending does not notify again on every lerd-ui restart.
func nginxUpdateTransition(statusJSON []byte) bool {
	if len(statusJSON) == 0 {
		return false
	}
	var snapshot struct {
		Nginx struct {
			UpdateAvailable bool `json:"update_available"`
		} `json:"nginx"`
	}
	if err := json.Unmarshal(statusJSON, &snapshot); err != nil {
		fmt.Printf("[notify-update] status decode: %v\n", err)
		return false
	}
	cur := snapshot.Nginx.UpdateAvailable

	nginxUpdateMu.Lock()
	prev := nginxUpdatePrev
	nginxUpdatePrev = &cur
	nginxUpdateMu.Unlock()

	return prev != nil && !*prev && cur
}

func notificationForNginxUpdate() push.Notification {
	return push.Notification{
		Kind:     "update_available",
		TitleKey: "notify_update_title",
		Title:    "Update available: Nginx",
		BodyKey:  "notify_nginx_update_body",
		Body:     "A newer nginx image is published. Update to pick it up.",
		Params:   map[string]string{"service": "Nginx"},
		Tag:      "lerd-update-nginx",
		URL:      "#system/nginx",
		Data:     map[string]string{"service": "Nginx"},
		Urgency:  "low",
		TTL:      3600,
	}
}
