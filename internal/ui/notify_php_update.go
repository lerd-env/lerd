package ui

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/geodro/lerd/internal/push"
)

var (
	phpUpdatesMu   sync.Mutex
	phpUpdatesPrev map[string]bool
)

// notifyOnPHPBaseUpdates dispatches one notification per PHP version whose
// base image has just been found stale, so a republished base is announced the
// same way a service update is.
func notifyOnPHPBaseUpdates(statusJSON []byte) {
	for _, version := range phpBaseUpdateTransitions(statusJSON) {
		dispatchNotification(notificationForPHPBaseUpdate(version))
	}
}

// phpBaseUpdateTransitions returns the versions whose update_available flag
// flipped false → true against the previous status snapshot, and records the
// new state. The first snapshot of a process seeds that state and reports
// nothing: everything already stale would otherwise notify on startup.
func phpBaseUpdateTransitions(statusJSON []byte) []string {
	if len(statusJSON) == 0 {
		return nil
	}
	var snapshot struct {
		PHPFPMs []struct {
			Version         string `json:"version"`
			UpdateAvailable bool   `json:"update_available"`
		} `json:"php_fpms"`
	}
	if err := json.Unmarshal(statusJSON, &snapshot); err != nil {
		fmt.Printf("[notify-update] status decode: %v\n", err)
		return nil
	}
	cur := make(map[string]bool, len(snapshot.PHPFPMs))
	for _, p := range snapshot.PHPFPMs {
		cur[p.Version] = p.UpdateAvailable
	}

	phpUpdatesMu.Lock()
	prev := phpUpdatesPrev
	phpUpdatesPrev = cur
	phpUpdatesMu.Unlock()

	if prev == nil {
		return nil
	}
	return newUpdatesAvailable(prev, cur)
}

func notificationForPHPBaseUpdate(version string) push.Notification {
	label := "PHP " + version
	return push.Notification{
		Kind:     "update_available",
		TitleKey: "notify_update_title",
		Title:    "Update available: " + label,
		BodyKey:  "notify_php_base_update_body",
		Body:     "A refreshed base image is published. Rebuild to pick it up.",
		Params:   map[string]string{"service": label, "version": version},
		Tag:      "lerd-update-php-" + version,
		URL:      "#system/php-" + version,
		Data:     map[string]string{"service": label, "version": version},
		Urgency:  "low",
		TTL:      3600,
	}
}
