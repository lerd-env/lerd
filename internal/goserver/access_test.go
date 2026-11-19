package goserver

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/idle"
	"github.com/geodro/lerd/internal/reqstats"
)

// Every request lands on the watcher's access feed in the format its parsers
// already read from nginx, so request stats and idle-suspend see the Go web
// server's traffic with no change on their side.
func TestAccessFeed_reportsWhatTheWatcherParses(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "access.sock")
	ln, err := net.ListenPacket("unixgram", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Lerd-Rid", "0123abcd0123abcd")
		w.WriteHeader(201)
	})
	h := &AccessFeed{Next: app, Target: "unix:" + sock}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "http://app.test/users?page=2", nil))

	buf := make([]byte, 2048)
	ln.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := ln.ReadFrom(buf)
	if err != nil {
		t.Fatalf("no datagram: %v", err)
	}
	rec, ok := reqstats.ParseAccessRecord(buf[:n])
	if !ok {
		t.Fatalf("request stats cannot parse %q", buf[:n])
	}
	if rec.Host != "app.test" || rec.Status != 201 || rec.Method != "POST" || rec.URI != "/users?page=2" || rec.RID != "0123abcd0123abcd" {
		t.Errorf("parsed %+v from %q", rec, buf[:n])
	}
	if host := idle.ParseAccessHost(buf[:n]); host != "app.test" {
		t.Errorf("idle tracker read host %q", host)
	}
}

// With no listener the request is still served; the feed is best effort, as
// nginx's syslog is.
func TestAccessFeed_missingListenerDoesNotFailTheRequest(t *testing.T) {
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	h := &AccessFeed{Next: app, Target: "unix:" + filepath.Join(t.TempDir(), "nobody.sock")}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://app.test/", nil))
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}
