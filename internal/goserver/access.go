package goserver

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AccessFeed reports every request to lerd's watcher in nginx's lerd_access
// line, so request stats and idle-suspend read it unchanged. Best effort: a
// missing listener never fails a request.
type AccessFeed struct {
	Next http.Handler
	// Target is config.AccessLogTarget(): "unix:/path" or "host:port" (UDP).
	Target string

	mu   sync.Mutex
	conn net.Conn
}

func (a *AccessFeed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	a.Next.ServeHTTP(rw, r)
	if rw.skip {
		return
	}

	rid := rw.Header().Get("X-Lerd-Rid")
	if rid == "" {
		rid = "-"
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	line := fmt.Sprintf("<190>lerdaccess: %s|%d|%.3f|%s|%s|%s",
		host, rw.status, time.Since(start).Seconds(), r.Method, rid, strings.ReplaceAll(r.URL.RequestURI(), " ", "%20"))
	a.send([]byte(line))
}

func (a *AccessFeed) send(b []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		network, addr := "udp", a.Target
		if path, ok := strings.CutPrefix(a.Target, "unix:"); ok {
			network, addr = "unixgram", path
		}
		conn, err := net.Dial(network, addr)
		if err != nil {
			return
		}
		a.conn = conn
	}
	if _, err := a.conn.Write(b); err != nil {
		// The watcher restarted and rebound its socket; dial again next time.
		a.conn.Close()
		a.conn = nil
	}
}

// statusWriter remembers the status the app answered with.
type statusWriter struct {
	http.ResponseWriter
	status int
	skip   bool
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush keeps streamed responses (server-sent events, Vite) streaming.
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
