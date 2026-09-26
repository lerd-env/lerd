package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type dashWakeStub struct {
	pings, wakes atomic.Int32
	ready        atomic.Bool
}

func stubDashWake(t *testing.T, asleep bool) *dashWakeStub {
	t.Helper()
	prevPing, prevAsleep, prevWake, prevUp := dashPing, dashAsleep, dashWake, dashUpstream
	t.Cleanup(func() {
		dashPing, dashAsleep, dashWake, dashUpstream = prevPing, prevAsleep, prevWake, prevUp
		dashLastPing = map[string]time.Time{}
	})
	dashLastPing = map[string]time.Time{}
	s := &dashWakeStub{}
	dashPing = func(string) { s.pings.Add(1) }
	dashAsleep = func(string) bool { return asleep }
	dashWake = func(string) error { s.wakes.Add(1); return nil }
	dashUpstream = func(*url.URL) bool { return s.ready.Load() }
	return s
}

var dashTarget, _ = url.Parse("http://localhost:8025")

func dashRequest(dest string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/_svc/mailpit/", nil)
	if dest != "" {
		r.Header.Set("Sec-Fetch-Dest", dest)
	}
	return r
}

func TestServeWhileWaking_awakeServiceGoesStraightThrough(t *testing.T) {
	s := stubDashWake(t, false)
	w := httptest.NewRecorder()
	if !serveWhileWaking(w, dashRequest("iframe"), "mailpit", dashTarget) {
		t.Fatal("an awake service was held")
	}
	if s.wakes.Load() != 0 || s.pings.Load() != 1 {
		t.Fatalf("wakes=%d pings=%d", s.wakes.Load(), s.pings.Load())
	}
}

// A page load gets the waking page at once rather than waiting on the wake.
func TestServeWhileWaking_pageLoadGetsTheWakingPage(t *testing.T) {
	s := stubDashWake(t, true)
	w := httptest.NewRecorder()
	if serveWhileWaking(w, dashRequest("iframe"), "mailpit", dashTarget) {
		t.Fatal("proxied a page load to a sleeping upstream")
	}
	if body := w.Body.String(); !strings.Contains(body, "Waking up") || !strings.Contains(body, "mailpit") || !strings.Contains(body, `http-equiv="refresh"`) {
		t.Fatalf("not the waking page:\n%s", body)
	}
	s.ready.Store(true)
	waitFor(t, func() bool { return s.wakes.Load() == 1 && !wakeInProgress("mailpit") })
}

func wakeInProgress(name string) bool {
	dashWakeMu.Lock()
	defer dashWakeMu.Unlock()
	_, ok := dashWaking[name]
	return ok
}

// The page's own requests wait for the dashboard to answer, sharing one wake.
func TestServeWhileWaking_assetsWaitForTheDashboardToAnswer(t *testing.T) {
	s := stubDashWake(t, true)
	results := make(chan bool, 3)
	for range 3 {
		go func() {
			results <- serveWhileWaking(httptest.NewRecorder(), dashRequest("script"), "mailpit", dashTarget)
		}()
	}
	select {
	case <-results:
		t.Fatal("an asset went through before the dashboard answered")
	case <-time.After(150 * time.Millisecond):
	}
	s.ready.Store(true)
	for range 3 {
		if !<-results {
			t.Fatal("asset not let through once the dashboard answered")
		}
	}
	if s.wakes.Load() != 1 {
		t.Fatalf("wakes = %d, want one shared wake", s.wakes.Load())
	}
	waitFor(t, func() bool { return !wakeInProgress("mailpit") })
}

func TestIsPageLoad(t *testing.T) {
	for dest, want := range map[string]bool{"document": true, "iframe": true, "script": false, "empty": false} {
		if got := isPageLoad(dashRequest(dest)); got != want {
			t.Errorf("Sec-Fetch-Dest %s: %v, want %v", dest, got, want)
		}
	}
	r := dashRequest("")
	r.Header.Set("Accept", "text/html,application/xhtml+xml")
	if !isPageLoad(r) {
		t.Error("a browser without fetch metadata loading HTML is a page load")
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition never met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDashboardKeepAlive_countsAsUseOfAKnownService(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	prev := serviceKeepAlivePing
	t.Cleanup(func() { serviceKeepAlivePing = prev })
	var pinged []string
	serviceKeepAlivePing = func(k string) { pinged = append(pinged, k) }

	for name, want := range map[string]int{"mailpit": http.StatusNoContent, "nothing-here": http.StatusNotFound} {
		w := httptest.NewRecorder()
		handleDashboardKeepAlive(w, httptest.NewRequest(http.MethodPost, "/api/dashboard/keepalive?name="+name, nil))
		if w.Code != want {
			t.Errorf("%s: %d, want %d", name, w.Code, want)
		}
	}
	if len(pinged) != 1 || pinged[0] != "svc:mailpit" {
		t.Fatalf("pinged %v, want only the known service", pinged)
	}
}
