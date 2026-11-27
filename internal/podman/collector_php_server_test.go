package podman

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// runBridgePHPServer serves page from php -S, one process across requests the
// way an FPM worker is, beside the bridge file it requires, and returns the
// lines and connections that file sent for that many requests.
func runBridgePHPServer(t *testing.T, file string, page string, requests int, dropEach bool) ([]string, int) {
	t.Helper()
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	dir := t.TempDir()
	src, err := dumpBridgeFS.ReadFile("dumpbridge/" + file)
	if err != nil {
		t.Fatal(err)
	}
	bridgePath := filepath.Join(dir, file)
	page = "<?php require_once " + phpQuote(bridgePath) + "; " + page
	for path, body := range map[string]string{bridgePath: string(src), filepath.Join(dir, "index.php"): page} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, _ := exec.Command(php, "-n", "-r", "echo file_exists("+phpQuote(bridgePath)+") ? 'Y' : 'N';").Output(); string(out) != "Y" {
		t.Skip("php cannot read host files (containerised/sandboxed wrapper); native php needed")
	}
	sink := listenCapture(t, dropEach)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	srv := exec.Command(php, "-n", "-d", "auto_prepend_file=", "-d", "lerd.devtools_host=unix://"+sink.sock, "-S", addr, "-t", dir)
	srv.Env = append(os.Environ(), "LERD_DEVTOOLS_HOST=unix://"+sink.sock,
		"LERD_DEVTOOLS_SEAMS="+filepath.Join(dir, "devtools-seams.conf"))
	if err := srv.Start(); err != nil {
		t.Fatalf("php -S: %v", err)
	}
	t.Cleanup(func() { _ = srv.Process.Kill(); _ = srv.Wait() })
	for i := 0; i < requests; i++ {
		var resp *http.Response
		for try := 0; try < 50; try++ {
			if resp, err = http.Get("http://" + addr + "/"); err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if err != nil {
			t.Fatalf("GET %d: %v", i, err)
		}
		resp.Body.Close()
		// Lets a dropping sink close the connection before the next request.
		time.Sleep(100 * time.Millisecond)
	}
	return sink.result()
}

func runCollectorPHPServer(t *testing.T, requests int, dropEach bool) ([]string, int) {
	return runBridgePHPServer(t, "devtools-collector.php", `\Lerd\Collector\send_text('hit');`, requests, dropEach)
}

// On macOS the bridge reaches lerd-ui over TCP, and a connection opened and
// closed per request left a port in TIME_WAIT for each one: at a few hundred
// requests a second that ran the Mac out of ports. A worker keeps one instead.
func TestCollectorPHP_KeepsOneConnectionAcrossRequests(t *testing.T) {
	lines, conns := runCollectorPHPServer(t, 3, false)
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3: %v", len(lines), lines)
	}
	if conns != 1 {
		t.Errorf("connections = %d, want 1 for the worker's three requests", conns)
	}
}

// lerd-ui ends a connection that sits idle between requests, and the next
// request has to open a fresh one rather than lose its event.
func TestCollectorPHP_ReopensAcrossRequestsAfterAnIdleClose(t *testing.T) {
	lines, conns := runCollectorPHPServer(t, 3, true)
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want all 3 despite each connection closing: %v", len(lines), lines)
	}
	if conns != 3 {
		t.Errorf("connections = %d, want 3", conns)
	}
}

// The Laravel adapter has its own sender, which the extension uses for a
// Laravel app's queries and views, so it keeps its connection the same way.
func TestLaravelAdapterPHP_KeepsOneConnectionAcrossRequests(t *testing.T) {
	lines, conns := runBridgePHPServer(t, "laravel-adapter.php", `\Lerd\LaravelAdapter\send(['kind' => 'hit']);`, 3, false)
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3: %v", len(lines), lines)
	}
	if conns != 1 {
		t.Errorf("connections = %d, want 1 for the worker's three requests", conns)
	}
}

func TestLaravelAdapterPHP_ReopensAcrossRequestsAfterAnIdleClose(t *testing.T) {
	lines, conns := runBridgePHPServer(t, "laravel-adapter.php", `\Lerd\LaravelAdapter\send(['kind' => 'hit']);`, 3, true)
	if len(lines) != 3 || conns != 3 {
		t.Errorf("got %d lines over %d connections, want 3 over 3: %v", len(lines), conns, lines)
	}
}
