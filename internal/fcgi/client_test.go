package fcgi

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// serveFCGI runs Go's own FastCGI responder as the PHP-FPM stand-in, so the
// client is checked against an independent implementation of the protocol.
func serveFCGI(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go fcgi.Serve(ln, h)
	return ln.Addr().String()
}

func TestServe_passesMethodBodyAndParams(t *testing.T) {
	addr := serveFCGI(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		env := fcgi.ProcessEnv(r)
		w.Header().Set("X-Seen", "yes")
		fmt.Fprintf(w, "%s %s %s %s", r.Method, body, env["SCRIPT_FILENAME"], r.Header.Get("X-Custom"))
	})
	req := httptest.NewRequest("POST", "http://app.test/form?a=1", strings.NewReader("hello"))
	req.Header.Set("X-Custom", "c1")
	rec := httptest.NewRecorder()

	err := Serve(rec, req, addr, map[string]string{"SCRIPT_FILENAME": "/srv/app/public/index.php"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || rec.Header().Get("X-Seen") != "yes" {
		t.Fatalf("status %d headers %v", rec.Code, rec.Header())
	}
	if got, want := rec.Body.String(), "POST hello /srv/app/public/index.php c1"; got != want {
		t.Errorf("body %q, want %q", got, want)
	}
}

// PHP sets its status through a Status header, and a Location without one is
// a redirect, both per the CGI spec nginx follows.
func TestServe_honoursStatusAndLocation(t *testing.T) {
	cases := []struct {
		name   string
		h      http.HandlerFunc
		status int
		loc    string
	}{
		{"status header", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(418) }, 418, ""},
		{"redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/login", 302) }, 302, "/login"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addr := serveFCGI(t, c.h)
			rec := httptest.NewRecorder()
			if err := Serve(rec, httptest.NewRequest("GET", "http://app.test/", nil), addr, nil, 0); err != nil {
				t.Fatal(err)
			}
			if rec.Code != c.status || rec.Header().Get("Location") != c.loc {
				t.Errorf("got %d %q, want %d %q", rec.Code, rec.Header().Get("Location"), c.status, c.loc)
			}
		})
	}
}

// A large body crosses many 64KiB records each way.
func TestServe_streamsLargeBodies(t *testing.T) {
	addr := serveFCGI(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	})
	big := strings.Repeat("x", 300_000)
	rec := httptest.NewRecorder()
	if err := Serve(rec, httptest.NewRequest("PUT", "http://app.test/", strings.NewReader(big)), addr, nil, 0); err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() != len(big) {
		t.Errorf("echoed %d bytes, want %d", rec.Body.Len(), len(big))
	}
}

// Nothing listening is an error the caller turns into a 502, not a hang.
func TestServe_unreachableUpstreamErrors(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	if err := Serve(httptest.NewRecorder(), httptest.NewRequest("GET", "http://app.test/", nil), addr, nil, 0); err == nil {
		t.Fatal("expected an error for a closed upstream")
	}
}

// A responder silent for longer than the timeout is ErrTimeout, which the web
// server answers with 504 instead of holding the browser.
func TestServe_silentResponderTimesOut(t *testing.T) {
	addr := serveFCGI(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte("late"))
	})
	start := time.Now()
	err := Serve(httptest.NewRecorder(), httptest.NewRequest("GET", "http://app.test/", nil), addr, nil, 300*time.Millisecond)
	if !errors.Is(err, ErrTimeout) || time.Since(start) > 600*time.Millisecond {
		t.Fatalf("err %v after %v, want ErrTimeout within the timeout", err, time.Since(start))
	}
}
