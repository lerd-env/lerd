package podman

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCollectorPHP_FiltersAndExtracts runs the real devtools-collector.php under
// the host php and captures what it ships over the socket, so the pure-PHP
// filter/extract logic (event noise filter, Messenger Envelope unwrap, http
// method+url) is covered without a Laravel/Symfony app. Skipped where php isn't
// installed (e.g. minimal CI images).
func TestCollectorPHP_FiltersAndExtracts(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}

	collector, err := DevtoolsCollectorPHP()
	if err != nil {
		t.Fatalf("DevtoolsCollectorPHP: %v", err)
	}
	dir := t.TempDir()
	collectorPath := filepath.Join(dir, "devtools-collector.php")
	if err := os.WriteFile(collectorPath, []byte(collector), 0o644); err != nil {
		t.Fatalf("write collector: %v", err)
	}

	// On dev boxes `php` is often lerd's container wrapper, which runs in an
	// FPM container that can't see the host's temp dir or socket. Detect that
	// (and any sandboxed php) by checking it can read a host file; skip if not,
	// since the harness needs a native php.
	preflight := filepath.Join(dir, "preflight.php")
	if err := os.WriteFile(preflight, []byte("<?php echo file_exists("+phpQuote(collectorPath)+") ? 'Y' : 'N';"), 0o644); err != nil {
		t.Fatalf("write preflight: %v", err)
	}
	if out, _ := exec.Command(php, preflight).CombinedOutput(); !strings.Contains(string(out), "Y") {
		t.Skip("php cannot read host files (containerised/sandboxed wrapper); native php needed")
	}

	sock := filepath.Join(dir, "c.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var mu sync.Mutex
	var lines []string
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			b, _ := io.ReadAll(conn)
			conn.Close()
			if s := strings.TrimSpace(string(b)); s != "" {
				mu.Lock()
				lines = append(lines, s)
				mu.Unlock()
			}
		}
	}()

	// A Messenger Envelope stub so the unwrap branch (Envelope -> inner message
	// class) is exercised, plus an app message class.
	script := `<?php
namespace Symfony\Component\Messenger { class Envelope { private $m; function __construct($m){ $this->m = $m; } function getMessage(){ return $this->m; } } }
namespace App\Message { class SendInvoice {} }
namespace {
    require ` + phpQuote(collectorPath) + `;
    \Lerd\Collector\event(new \stdClass(), 'kernel.request');                 // framework noise -> dropped
    \Lerd\Collector\event(new \stdClass(), 'App\\Domain\\OrderPlaced');       // app event -> emitted
    \Lerd\Collector\http('GET', 'https://api.test/widgets');                  // emitted
    \Lerd\Collector\job(new \stdClass());                                     // raw message -> class stdClass
    \Lerd\Collector\job(new \Symfony\Component\Messenger\Envelope(new \App\Message\SendInvoice())); // unwrap to inner class
}
`
	scriptPath := filepath.Join(dir, "probe.php")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	cmd := exec.Command(php, scriptPath)
	cmd.Env = append(os.Environ(), "LERD_DEVTOOLS_HOST=unix://"+sock)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("php run failed: %v\n%s", err, out)
	}
	time.Sleep(200 * time.Millisecond) // let the accept loop drain

	mu.Lock()
	got := append([]string(nil), lines...)
	mu.Unlock()

	type ev struct {
		Kind string `json:"kind"`
		Data struct {
			Name   string `json:"name"`
			URL    string `json:"url"`
			Method string `json:"method"`
			Class  string `json:"class"`
		} `json:"data"`
	}
	var events, https, jobs []ev
	for _, line := range got {
		var e ev
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", line, err)
		}
		switch e.Kind {
		case "event":
			events = append(events, e)
		case "http":
			https = append(https, e)
		case "job":
			jobs = append(jobs, e)
		}
	}

	// Noise event must be dropped; the app event must survive.
	for _, e := range events {
		if e.Data.Name == "kernel.request" {
			t.Errorf("framework-internal event leaked through the filter")
		}
	}
	if len(events) != 1 || events[0].Data.Name != "App\\Domain\\OrderPlaced" {
		t.Errorf("events = %+v, want one App\\Domain\\OrderPlaced", events)
	}
	if len(https) != 1 || https[0].Data.URL != "https://api.test/widgets" || https[0].Data.Method != "GET" {
		t.Errorf("http = %+v, want GET https://api.test/widgets", https)
	}
	// Raw stdClass kept as-is; the Envelope unwrapped to its inner message class.
	classes := map[string]bool{}
	for _, j := range jobs {
		classes[j.Data.Class] = true
	}
	if !classes["stdClass"] || !classes["App\\Message\\SendInvoice"] {
		t.Errorf("job classes = %v, want stdClass + App\\Message\\SendInvoice (Envelope unwrapped)", classes)
	}
}

// phpQuote single-quotes a path for embedding in a PHP require.
func phpQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}
