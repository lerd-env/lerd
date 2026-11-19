package goserver

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// wakeFixture is a host-proxy site idle-suspended by lerd, with lerd-ui's
// hold played by a server that reports what it was handed and answers status.
func wakeFixture(t *testing.T, status int) (*Handler, *string) {
	t.Helper()
	h, _ := fixture(t)
	seen := new(string)
	ui := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*seen = fmt.Sprintf("%s %s host=%s uri=%s scheme=%s body=%s", r.Method, r.URL.Path,
			r.Header.Get("X-Lerd-Wake-Host"), r.Header.Get("X-Lerd-Wake-Uri"), r.Header.Get("X-Lerd-Wake-Scheme"), body)
		w.WriteHeader(status)
		fmt.Fprint(w, "app answered after waking")
	}))
	t.Cleanup(ui.Close)
	site := h.Sites()[0]
	site.HostPort = 5173
	site.IdleSuspendedWorkers = []string{config.HostProxyWorkerName}
	h.Sites = func() []config.Site { return []config.Site{site} }
	h.WakeHold, _ = url.Parse(ui.URL)
	h.Proxy = func(config.Site) (*url.URL, bool) {
		t.Fatal("a sleeping site was proxied to its dev server")
		return nil, false
	}
	return h, seen
}

// The request is held in lerd-ui whole, method and body included, and the
// app's own answer comes back once the site is awake.
func TestWake_holdsTheWholeRequestAndReturnsTheAppsAnswer(t *testing.T) {
	h, seen := wakeFixture(t, 201)
	rec := get(h, "POST", "http://app.test/webhook?id=7", "payload")
	if rec.Code != 201 || rec.Body.String() != "app answered after waking" {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
	if want := "POST " + WakeHoldPath + " host=app.test uri=/webhook?id=7 scheme=http body=payload"; *seen != want {
		t.Errorf("hold saw %q, want %q", *seen, want)
	}
}

// Only a failure of the hold itself turns into the waking page.
func TestWake_holdFailureServesTheWakingPage(t *testing.T) {
	h, _ := wakeFixture(t, 599)
	os.MkdirAll(config.PausedDir(), 0o755)
	os.WriteFile(filepath.Join(config.PausedDir(), "waking.html"), []byte("<h1>Waking up</h1>"), 0o644)
	rec := get(h, "GET", "http://app.test/", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Waking up") {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
}

func TestWake_unreachableHoldServesTheWakingPage(t *testing.T) {
	h, _ := wakeFixture(t, 200)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	h.WakeHold, _ = url.Parse("http://" + ln.Addr().String())
	ln.Close()
	os.MkdirAll(config.PausedDir(), 0o755)
	os.WriteFile(filepath.Join(config.PausedDir(), "waking.html"), []byte("<h1>Waking up</h1>"), 0o644)
	if rec := get(h, "GET", "http://app.test/", ""); !strings.Contains(rec.Body.String(), "Waking up") {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
}

// A held request is not the app's, so it stays off the access feed; the hold
// reports the activity itself.
func TestWake_heldRequestsStayOffTheAccessFeed(t *testing.T) {
	h, _ := wakeFixture(t, 200)
	sock := filepath.Join(t.TempDir(), "a.sock")
	ln, err := net.ListenPacket("unixgram", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	feed := &AccessFeed{Next: h, Target: "unix:" + sock}
	feed.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://app.test/", nil))
	ln.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if n, _, err := ln.ReadFrom(make([]byte, 512)); err == nil {
		t.Fatalf("held request reached the feed (%d bytes)", n)
	}
}

// An awake site never goes near the hold.
func TestWake_awakeSiteIsServedDirectly(t *testing.T) {
	h, seen := wakeFixture(t, 200)
	site := h.Sites()[0]
	site.IdleSuspendedWorkers = nil
	site.HostPort = 0
	h.Sites = func() []config.Site { return []config.Site{site} }
	h.Proxy = nil
	get(h, "GET", "http://app.test/", "")
	if *seen != "" {
		t.Fatalf("awake site went to the hold: %q", *seen)
	}
}
