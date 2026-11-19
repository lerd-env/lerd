package goserver

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// workerSite is a PHP site whose "ws" worker is served by an httptest server
// standing in for the worker inside the PHP container.
func workerSite(t *testing.T, proxy *config.WorkerProxy) (*Handler, config.Site) {
	t.Helper()
	h, _ := fixture(t)
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "worker host=%s uri=%s xfp=%s", r.Host, r.URL.RequestURI(), r.Header.Get("X-Forwarded-Proto"))
	}))
	t.Cleanup(worker.Close)
	_, portStr, _ := net.SplitHostPort(strings.TrimPrefix(worker.URL, "http://"))
	proxy.DefaultPort, _ = strconv.Atoi(portStr)

	site := h.Sites()[0]
	site.Framework = "laravel"
	if err := os.WriteFile(filepath.Join(site.Path, ".lerd.yaml"), []byte("framework: laravel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil {
		t.Fatal(err)
	}
	proj.CustomWorkers = map[string]config.FrameworkWorker{"ws": {Command: "php artisan ws:serve", Proxy: proxy}}
	if err := config.SaveProjectConfig(site.Path, proj); err != nil {
		t.Fatal(err)
	}
	h.Sites = func() []config.Site { return []config.Site{site} }
	h.WorkerHost = func(config.Site, bool) string { return "127.0.0.1" }
	return h, site
}

func TestWorker_pathIsProxiedAndAnchored(t *testing.T) {
	h, _ := workerSite(t, &config.WorkerProxy{Path: "/app"})
	for _, path := range []string{"/app", "/app/socket?x=1"} {
		if body := get(h, "GET", "http://app.test"+path, "").Body.String(); !strings.HasPrefix(body, "worker host=app.test uri="+path) {
			t.Errorf("%s: %q", path, body)
		}
	}
	// A route merely sharing the prefix belongs to the app, not the worker.
	if body := get(h, "GET", "http://app.test/appfoo", "").Body.String(); strings.HasPrefix(body, "worker") {
		t.Errorf("/appfoo reached the worker: %q", body)
	}
}

func TestWorker_subdomainTakesTheWholeHost(t *testing.T) {
	h, _ := workerSite(t, &config.WorkerProxy{Subdomain: "docs"})
	if body := get(h, "GET", "http://docs.app.test/anything", "").Body.String(); !strings.HasPrefix(body, "worker host=docs.app.test uri=/anything") {
		t.Errorf("subdomain body %q", body)
	}
	// Any other subdomain is still the site.
	if body := get(h, "GET", "http://www.app.test/", "").Body.String(); strings.HasPrefix(body, "worker") {
		t.Errorf("www reached the worker: %q", body)
	}
}

// An invalid label is refused, as nginx refuses it, leaving the host to the site.
func TestWorker_invalidSubdomainIsNotServed(t *testing.T) {
	h, _ := workerSite(t, &config.WorkerProxy{Subdomain: "Docs_x"})
	if body := get(h, "GET", "http://Docs_x.app.test/", "").Body.String(); strings.HasPrefix(body, "worker") {
		t.Errorf("invalid label reached the worker: %q", body)
	}
}

// Vite HMR and Reverb ride a WebSocket, so an upgrade through a worker path
// has to reach the worker and carry bytes both ways.
func TestWorker_websocketUpgradePassesThrough(t *testing.T) {
	h, site := workerSite(t, &config.WorkerProxy{Path: "/app"})
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			http.Error(w, "no upgrade", 400)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		rw.Flush()
		line, _ := rw.ReadString('\n')
		rw.WriteString("echo:" + line)
		rw.Flush()
	}))
	t.Cleanup(echo.Close)
	_, portStr, _ := net.SplitHostPort(strings.TrimPrefix(echo.URL, "http://"))
	site.WorkerPorts = nil
	proj, _ := config.LoadProjectConfig(site.Path)
	p := proj.CustomWorkers["ws"]
	p.Proxy.DefaultPort, _ = strconv.Atoi(portStr)
	proj.CustomWorkers["ws"] = p
	config.SaveProjectConfig(site.Path, proj)

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprint(conn, "GET /app/socket HTTP/1.1\r\nHost: app.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("no switch: %v %v", resp, err)
	}
	fmt.Fprint(conn, "ping\n")
	if line, err := br.ReadString('\n'); err != nil || line != "echo:ping\n" {
		t.Fatalf("echo %q %v", line, err)
	}
}
