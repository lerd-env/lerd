package ui

import (
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/eventbus"
	"github.com/geodro/lerd/internal/push"
	"github.com/geodro/lerd/internal/serviceops"
)

// externalDBPollInterval is how soon a server going away or coming back shows.
// A TCP connect is cheap, and lerd has no other way to hear about a server it
// does not run.
const externalDBPollInterval = 15 * time.Second

// externalDBWatch remembers which external servers answered last time. A server
// first seen down stays quiet: one the user left off would otherwise notify on
// every start of the dashboard.
type externalDBWatch struct {
	up map[string]bool
}

// observe takes this round's answers and returns the servers that just stopped
// answering, and whether anything changed at all, a server coming or going included.
func (w *externalDBWatch) observe(now map[string]bool) (wentDown []string, changed bool) {
	changed = w.up != nil && len(now) != len(w.up)
	for name, up := range now {
		was, known := w.up[name]
		if !known {
			changed = changed || w.up != nil
			continue
		}
		if was == up {
			continue
		}
		changed = true
		if !up {
			wentDown = append(wentDown, name)
		}
	}
	w.up = now
	return wentDown, changed
}

func notificationForExternalDBOffline(name, address string) push.Notification {
	return push.Notification{
		Kind:     "worker_failed",
		TitleKey: "notify_external_db_offline_title",
		Title:    name + " is offline",
		BodyKey:  "notify_external_db_offline_body",
		Body:     "The server at " + address + " stopped answering.",
		Params:   map[string]string{"name": name, "address": address},
		Tag:      "lerd-external-" + name,
		URL:      "#services/" + name,
		Urgency:  "high",
		TTL:      300,
	}
}

func externalDBStates() (map[string]bool, map[string]string) {
	states, addresses := map[string]bool{}, map[string]string{}
	customs, err := config.ListCustomServices()
	if err != nil {
		return states, addresses
	}
	for _, svc := range customs {
		if svc.External != nil {
			states[svc.Name] = serviceops.ExternalDBReachable(*svc.External)
			addresses[svc.Name] = svc.External.Address()
		}
	}
	return states, addresses
}

func runExternalDBWatcher() {
	var watch externalDBWatch
	for range time.Tick(externalDBPollInterval) {
		states, addresses := externalDBStates()
		wentDown, changed := watch.observe(states)
		for _, name := range wentDown {
			dispatchNotification(notificationForExternalDBOffline(name, addresses[name]))
		}
		if changed {
			eventbus.Default.Publish(eventbus.KindServices)
		}
	}
}
