package ui

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/activityping"
	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

// The seams dashboard waking goes through, vars so tests stand in for the
// watcher, the containers and the upstream.
var (
	dashPing     = activityping.Site
	dashAsleep   = config.ServiceIsIdleSuspended
	dashWake     = serviceops.WakeService
	dashUpstream = func(u *url.URL) bool { return serviceops.DashboardAnswers(u.String()) }
	dashWakeMax  = 60 * time.Second
)

// dashPingEvery bounds the activity pings: a dashboard page fires a burst of
// asset requests, and one ping per burst keeps the service awake just as well.
const dashPingEvery = 5 * time.Second

var (
	dashWakeMu   sync.Mutex
	dashLastPing = map[string]time.Time{}
	// dashWaking holds one channel per service being woken, closed once its
	// dashboard answers, so every request of a burst shares a single wake.
	dashWaking = map[string]chan struct{}{}
)

// serveWhileWaking keeps a proxied dashboard's service awake under
// idle-suspend and reports whether the request may go on to the upstream. When
// the service is asleep it starts the wake in the background: a page load gets
// the waking page, which reloads itself onto the dashboard, and anything else
// (the page's own assets and API calls) waits for the wake instead of 502ing.
func serveWhileWaking(w http.ResponseWriter, r *http.Request, name string, target *url.URL) bool {
	done := dashWakeFor(name, target)
	if done == nil {
		return true
	}
	if isPageLoad(r) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(cli.WakingPage(name)))
		return false
	}
	select {
	case <-done:
		return true
	case <-r.Context().Done():
		return false
	}
}

// dashWakeFor pings the watcher (throttled) and, when idle-suspend has the
// service asleep, returns the channel its wake closes, starting that wake if
// none is running. nil means the service is up.
func dashWakeFor(name string, target *url.URL) <-chan struct{} {
	dashWakeMu.Lock()
	defer dashWakeMu.Unlock()
	if ch, ok := dashWaking[name]; ok {
		return ch
	}
	if time.Since(dashLastPing[name]) < dashPingEvery {
		return nil
	}
	dashPing("svc:" + name)
	dashLastPing[name] = time.Now()
	if !dashAsleep(name) {
		return nil
	}
	ch := make(chan struct{})
	dashWaking[name] = ch
	go func() {
		// A failed wake still releases the waiters; the upstream then answers
		// with its own error rather than the request hanging.
		if dashWake(name) == nil {
			deadline := time.Now().Add(dashWakeMax)
			for !dashUpstream(target) && time.Now().Before(deadline) {
				time.Sleep(100 * time.Millisecond)
			}
		}
		dashWakeMu.Lock()
		delete(dashWaking, name)
		dashWakeMu.Unlock()
		close(ch)
	}()
	return ch
}

// isPageLoad reports whether the browser is loading a page (top level or the
// dashboard's iframe) rather than fetching something for one.
func isPageLoad(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	switch r.Header.Get("Sec-Fetch-Dest") {
	case "document", "iframe":
		return true
	case "":
		return strings.Contains(r.Header.Get("Accept"), "text/html")
	}
	return false
}
