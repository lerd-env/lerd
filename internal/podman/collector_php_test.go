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
// runCollectorPHP writes the real devtools-collector.php next to a probe
// script, runs it under the host php, and returns every JSON line the script
// shipped over the capture socket. `body` is spliced into the probe after the
// collector is required; COLLECTOR is replaced with the collector's path.
// Skipped where php isn't installed or can't reach host files (e.g. lerd's own
// container wrapper on a dev box).
func runCollectorPHP(t *testing.T, body string) []string {
	t.Helper()
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

	script := strings.ReplaceAll(body, "COLLECTOR", phpQuote(collectorPath))
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
	defer mu.Unlock()
	return append([]string(nil), lines...)
}

// TestCollectorPHP_FiltersAndExtracts runs the real devtools-collector.php under
// the host php and captures what it ships over the socket, so the pure-PHP
// filter/extract logic (event noise filter, Messenger Envelope unwrap, http
// method+url) is covered without a Laravel/Symfony app.
func TestCollectorPHP_FiltersAndExtracts(t *testing.T) {
	// A Messenger Envelope stub so the unwrap branch (Envelope -> inner message
	// class) is exercised, plus an app message class.
	got := runCollectorPHP(t, `<?php
namespace Symfony\Component\Messenger { class Envelope { private $m; function __construct($m){ $this->m = $m; } function getMessage(){ return $this->m; } } }
namespace App\Message { class SendInvoice {} }
namespace {
    require COLLECTOR;
    \Lerd\Collector\event(new \stdClass(), 'kernel.request');                 // framework noise -> dropped
    \Lerd\Collector\event(new \stdClass(), 'App\\Domain\\OrderPlaced');       // app event -> emitted
    \Lerd\Collector\http('GET', 'https://api.test/widgets');                  // emitted
    \Lerd\Collector\job(new \stdClass());                                     // raw message -> class stdClass
    \Lerd\Collector\job(new \Symfony\Component\Messenger\Envelope(new \App\Message\SendInvoice())); // unwrap to inner class
}
`)

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

// TestCollectorPHP_TagsTestRuns covers the ctx.test signal: PHPUnit's bootstrap
// constant is the only thing that separates a test run from any other CLI
// invocation, and the Debug lenses hide tagged events by default.
func TestCollectorPHP_TagsTestRuns(t *testing.T) {
	type ev struct {
		Ctx struct {
			Type string `json:"type"`
			Test bool   `json:"test"`
		} `json:"ctx"`
	}
	decode := func(t *testing.T, lines []string) ev {
		t.Helper()
		if len(lines) != 1 {
			t.Fatalf("got %d events, want 1: %v", len(lines), lines)
		}
		var e ev
		if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", lines[0], err)
		}
		return e
	}

	underTest := decode(t, runCollectorPHP(t, `<?php
define('PHPUNIT_COMPOSER_INSTALL', '/app/vendor/autoload.php');
require COLLECTOR;
\Lerd\Collector\http('GET', 'https://api.test/widgets');
`))
	if !underTest.Ctx.Test {
		t.Errorf("ctx.test = false under a PHPUnit run, want true")
	}
	if underTest.Ctx.Type != "cli" {
		t.Errorf("ctx.type = %q, want cli — the test flag must not replace the SAPI", underTest.Ctx.Type)
	}

	plain := decode(t, runCollectorPHP(t, `<?php
require COLLECTOR;
\Lerd\Collector\http('GET', 'https://api.test/widgets');
`))
	if plain.Ctx.Test {
		t.Errorf("ctx.test = true for a plain CLI invocation, want false")
	}
}

// TestCollectorPHP_StampsCommandForCLI covers ctx.command, the CLI counterpart
// of ctx.request: a console event has no route to name, so the bridge stamps
// the invocation and consumers like the N+1 warning have somewhere to point.
func TestCollectorPHP_StampsCommandForCLI(t *testing.T) {
	type ev struct {
		Ctx struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			Request string `json:"request"`
		} `json:"ctx"`
	}
	lines := runCollectorPHP(t, `<?php
$_SERVER['argv'] = ['/app/artisan', 'tinker', '--queue=high', '--execute=for ($i = 0; $i < 4; $i++) { DB::select("select 1"); }'];
require COLLECTOR;
\Lerd\Collector\http('GET', 'https://api.test/widgets');
`)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1: %v", len(lines), lines)
	}
	var e ev
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("bad JSON line %q: %v", lines[0], err)
	}
	if e.Ctx.Command != "artisan tinker --queue=high --execute=..." {
		t.Errorf("ctx.command = %q, want short arguments kept and long values elided", e.Ctx.Command)
	}
	if e.Ctx.Request != "" {
		t.Errorf("ctx.request = %q, want empty on a CLI invocation", e.Ctx.Request)
	}
	if e.Ctx.Type != "cli" {
		t.Errorf("ctx.type = %q, want cli", e.Ctx.Type)
	}
}

// TestCollectorPHP_AttributesPastComposerInstalledCode builds a project whose
// framework core is a Composer package installed outside vendor/, the layout
// Drupal uses, and checks the recorded source is the project's own code rather
// than the framework layer that issued the query.
func TestCollectorPHP_AttributesPastComposerInstalledCode(t *testing.T) {
	type ev struct {
		Src struct {
			File string `json:"file"`
		} `json:"src"`
	}
	lines := runCollectorPHP(t, `<?php
$root = __DIR__ . '/app';
@mkdir($root . '/vendor/composer', 0777, true);
@mkdir($root . '/core/lib', 0777, true);
@mkdir($root . '/modules/custom', 0777, true);
file_put_contents($root . '/vendor/composer/installed.php', '<?php return ' . var_export([
    'root' => ['install_path' => $root],
    'versions' => [
        'acme/core' => ['install_path' => $root . '/core'],
    ],
], true) . ';');
file_put_contents($root . '/core/lib/Db.php', '<?php function acme_query() { \Lerd\Collector\http("GET", "https://api.test/widgets"); }');
file_put_contents($root . '/modules/custom/Listing.php', '<?php function acme_listing() { acme_query(); }');

$_SERVER['DOCUMENT_ROOT'] = $root;
require COLLECTOR;
require $root . '/core/lib/Db.php';
require $root . '/modules/custom/Listing.php';
acme_listing();
`)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1: %v", len(lines), lines)
	}
	var e ev
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("bad JSON line %q: %v", lines[0], err)
	}
	if !strings.HasSuffix(e.Src.File, "/modules/custom/Listing.php") {
		t.Errorf("src.file = %q, want the project's own module, not the installed framework package", e.Src.File)
	}
}
