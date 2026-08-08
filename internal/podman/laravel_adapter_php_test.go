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
$_SERVER['argv'] = ['/app/artisan', 'queue:work', '--queue=high', '--execute=for ($i = 0; $i < 4; $i++) { DB::select("select 1"); }'];
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
	if e.Ctx.Command != "artisan queue:work --queue=high --execute=..." {
		t.Errorf("ctx.command = %q, want short arguments kept and long values elided", e.Ctx.Command)
	}
	if e.Ctx.Request != "" {
		t.Errorf("ctx.request = %q, want empty on a CLI invocation", e.Ctx.Request)
	}
	if e.Ctx.Type != "cli" {
		t.Errorf("ctx.type = %q, want cli", e.Ctx.Type)
	}
}

// TestLaravelAdapterPHP_SkipsViewsBladeCompiledForItself checks that a render
// whose template is the compiled artefact, which is what an inline or anonymous
// component resolves to, is left out, while a template in the project is kept.
func TestLaravelAdapterPHP_SkipsViewsBladeCompiledForItself(t *testing.T) {
	type ev struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	lines := runLaravelAdapterPHP(t, `<?php
define('LERD_DEVTOOLS_ON', true);
require ADAPTER;
$compiled = \Lerd\LaravelAdapter\compiled_view_dir(['config' => ['view.compiled' => '/app/storage/framework/views']]);
$renders = [
    '/app/storage/framework/views/dca1a29b69452d307c2c30a7b9cc0a6e.php' => '__components::dca1a29b69452d307c2c30a7b9cc0a6e',
    '/app/resources/views/app.blade.php'                                => 'app',
];
foreach ($renders as $path => $name) {
    if (!\Lerd\LaravelAdapter\is_synthetic_view($path, $compiled)) {
        \Lerd\LaravelAdapter\emit('view', ['name' => $name]);
    }
}
`)
	if len(lines) != 1 {
		t.Fatalf("got %d view events, want only the project's own template: %v", len(lines), lines)
	}
	var e ev
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("bad JSON line %q: %v", lines[0], err)
	}
	if e.Data.Name != "app" {
		t.Errorf("view name = %q, want app", e.Data.Name)
	}
}

// TestLaravelAdapterPHP_LabelsOnlyTheDevelopersViewData checks a view reports
// what it was passed, not what the factory shares with every view or what Blade
// adds to render it.
func TestLaravelAdapterPHP_LabelsOnlyTheDevelopersViewData(t *testing.T) {
	type ev struct {
		Data struct {
			Keys    []string          `json:"data_keys"`
			Preview map[string]string `json:"data_preview"`
		} `json:"data"`
	}
	lines := runLaravelAdapterPHP(t, `<?php
define('LERD_DEVTOOLS_ON', true);
require ADAPTER;
$data = [
    'page'            => [1, 2, 3, 4, 5, 6],
    'title'           => 'Dashboard',
    'app'             => new \stdClass(),        // shared with every view
    'errors'          => new \stdClass(),        // shared with every view
    '__env'           => new \stdClass(),        // Blade's own
    '__laravel_slots' => [],                     // Blade's own
    'componentName'   => 'app-layout',           // Blade's component machinery
    'attributes'      => new \stdClass(),
    'slot'            => new \stdClass(),
];
$shared = ['app' => null, 'errors' => null, '__env' => null];
$preview = \Lerd\LaravelAdapter\preview_data($data, $shared);
\Lerd\LaravelAdapter\emit('view', ['data_keys' => array_keys($preview), 'data_preview' => $preview]);
`)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1: %v", len(lines), lines)
	}
	var e ev
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("bad JSON line %q: %v", lines[0], err)
	}
	want := map[string]string{"page": "array(6)", "title": `"Dashboard"`}
	if len(e.Data.Preview) != len(want) {
		t.Fatalf("preview = %v, want only the developer's own variables %v", e.Data.Preview, want)
	}
	for k, v := range want {
		if e.Data.Preview[k] != v {
			t.Errorf("preview[%q] = %q, want %q", k, e.Data.Preview[k], v)
		}
	}
	if len(e.Data.Keys) != len(want) {
		t.Errorf("data_keys = %v, want the same set the preview reports", e.Data.Keys)
	}
}
