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

// runLaravelAdapterPHP writes the real laravel-adapter.php next to a probe
// script, runs it under the host php, and returns every JSON line the script
// shipped over the capture socket. `body` is spliced into the probe; ADAPTER is
// replaced with the adapter's path. Skipped where php isn't installed or is
// lerd's own container wrapper, like the collector harness.
func runLaravelAdapterPHP(t *testing.T, body string) []string {
	t.Helper()
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}

	adapter, err := dumpBridgeFS.ReadFile("dumpbridge/laravel-adapter.php")
	if err != nil {
		t.Fatalf("laravel adapter embed: %v", err)
	}
	dir := t.TempDir()
	adapterPath := filepath.Join(dir, "laravel-adapter.php")
	if err := os.WriteFile(adapterPath, adapter, 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	preflight := filepath.Join(dir, "preflight.php")
	if err := os.WriteFile(preflight, []byte("<?php echo file_exists("+phpQuote(adapterPath)+") ? 'Y' : 'N';"), 0o644); err != nil {
		t.Fatalf("write preflight: %v", err)
	}
	if out, _ := exec.Command(php, preflight).CombinedOutput(); !strings.Contains(string(out), "Y") {
		t.Skip("php cannot read host files (containerised/sandboxed wrapper); native php needed")
	}

	sock := filepath.Join(dir, "a.sock")
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

	script := strings.ReplaceAll(body, "ADAPTER", phpQuote(adapterPath))
	scriptPath := filepath.Join(dir, "probe.php")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	// The adapter resolves its target through get_cfg_var, not the env var.
	cmd := exec.Command(php, "-d", "lerd.devtools_host=unix://"+sock, scriptPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("php run failed: %v\n%s", err, out)
	}
	time.Sleep(200 * time.Millisecond) // let the accept loop drain

	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), lines...)
}

// TestLaravelAdapterPHP_StampsCommandForCLI pins that the adapter's own
// context() stamps ctx.command like the collector's does: Laravel query events
// are emitted here, not through the collector, so an artisan run only names its
// invocation if this copy stamps it too.
func TestLaravelAdapterPHP_StampsCommandForCLI(t *testing.T) {
	type ev struct {
		Kind string `json:"kind"`
		Ctx  struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			Request string `json:"request"`
		} `json:"ctx"`
	}
	lines := runLaravelAdapterPHP(t, `<?php
$_SERVER['argv'] = ['/app/artisan', 'queue:work', '--queue=high'];
define('LERD_DEVTOOLS_ON', true);
require ADAPTER;
\Lerd\LaravelAdapter\emit('query', ['sql' => 'select 1']);
`)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1: %v", len(lines), lines)
	}
	var e ev
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("bad JSON line %q: %v", lines[0], err)
	}
	if e.Kind != "query" {
		t.Errorf("kind = %q, want query", e.Kind)
	}
	if e.Ctx.Command != "artisan queue:work --queue=high" {
		t.Errorf("ctx.command = %q, want the script basename plus its arguments", e.Ctx.Command)
	}
	if e.Ctx.Request != "" {
		t.Errorf("ctx.request = %q, want empty on a CLI invocation", e.Ctx.Request)
	}
	if e.Ctx.Type != "cli" {
		t.Errorf("ctx.type = %q, want cli", e.Ctx.Type)
	}
}
