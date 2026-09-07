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

// fakeLaravelApp is the container shape the adapter reads at boot, plus a
// dispatcher that records what it listened to so a probe can fire events at it.
const fakeLaravelApp = `
class FakeEvents {
    public $cbs = [];
    public function listen($event, $cb) { $this->cbs[(string) $event][] = $cb; }
    public function fire($event, $arg = null) { foreach ($this->cbs[$event] ?? [] as $cb) { $cb($arg); } }
}
class FakeJob {
    private $queue; private $tries; private $id;
    public function __construct($queue, $tries, $id) { $this->queue = $queue; $this->tries = $tries; $this->id = $id; }
    public function resolveName() { return 'App\\Jobs\\SendInvoice'; }
    public function getQueue() { return $this->queue; }
    public function attempts() { return $this->tries; }
    public function uuid() { return $this->id; }
}
class FakeQueueEvent { public $job; public $queue; public $connectionName; public $exception; public $payload; }
class FakeMailEvent { public $message; }
function app() { return $GLOBALS['__lerd_fake_app']; }
$GLOBALS['__lerd_fake_app'] = ['events' => new FakeEvents()];
`

// TestLaravelAdapterPHP_JobLifecycle checks the adapter reports a job through
// every state a worker takes it, with the queue, attempt count and the time the
// job took, since a terminal-only report says nothing while a queue is draining.
func TestLaravelAdapterPHP_JobLifecycle(t *testing.T) {
	lines := runLaravelAdapterPHP(t, `<?php
`+fakeLaravelApp+`
define('LERD_DEVTOOLS_ON', true);
require ADAPTER;
$events = $GLOBALS['__lerd_fake_app']['events'];
$e = new FakeQueueEvent();
$e->job = new FakeJob('emails', 2, 'job-uuid-1');
$e->connectionName = 'redis';
$events->fire('Illuminate\\Queue\\Events\\JobProcessing', $e);
usleep(20000);
$events->fire('Illuminate\\Queue\\Events\\JobProcessed', $e);
$failed = new FakeQueueEvent();
$failed->job = $e->job;
$failed->connectionName = 'redis';
$failed->exception = new \RuntimeException('mailer refused the message');
$events->fire('Illuminate\\Queue\\Events\\JobFailed', $failed);
`)

	type ev struct {
		Data struct {
			Class      string  `json:"class"`
			Status     string  `json:"status"`
			Queue      string  `json:"queue"`
			UUID       string  `json:"uuid"`
			Attempts   int     `json:"attempts"`
			Connection string  `json:"connection"`
			TimeMS     float64 `json:"time_ms"`
			Exception  string  `json:"exception"`
		} `json:"data"`
	}
	if len(lines) != 3 {
		t.Fatalf("got %d events, want one per state: %v", len(lines), lines)
	}
	var events []ev
	for _, line := range lines {
		var e ev
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", line, err)
		}
		if e.Data.Class != "App\\Jobs\\SendInvoice" {
			t.Errorf("class = %q, want the resolved job name", e.Data.Class)
		}
		if e.Data.Queue != "emails" || e.Data.Attempts != 2 || e.Data.UUID != "job-uuid-1" {
			t.Errorf("queue/attempts/uuid = %q/%d/%q, want emails/2/job-uuid-1", e.Data.Queue, e.Data.Attempts, e.Data.UUID)
		}
		if e.Data.Connection != "redis" {
			t.Errorf("connection = %q, want redis", e.Data.Connection)
		}
		events = append(events, e)
	}
	for i, want := range []string{"processing", "processed", "failed"} {
		if events[i].Data.Status != want {
			t.Errorf("event %d status = %q, want %q", i, events[i].Data.Status, want)
		}
	}
	if events[1].Data.TimeMS <= 0 {
		t.Errorf("processed time_ms = %v, want the time the job took", events[1].Data.TimeMS)
	}
	if events[2].Data.Exception != "mailer refused the message" {
		t.Errorf("failed exception = %q, want the throwable's message", events[2].Data.Exception)
	}
}

// TestLaravelAdapterPHP_WorkerReportsJobsOnly checks the capture policy a queue
// worker runs under when full worker capture is off: its jobs are reported, and
// the rest of what it does is not registered at all.
func TestLaravelAdapterPHP_WorkerReportsJobsOnly(t *testing.T) {
	lines := runLaravelAdapterPHP(t, `<?php
`+fakeLaravelApp+`
define('LERD_DEVTOOLS_ON', false);
define('LERD_DEVTOOLS_JOBS', true);
require ADAPTER;
$events = $GLOBALS['__lerd_fake_app']['events'];
$e = new FakeQueueEvent();
$e->job = new FakeJob('emails', 1, 'job-uuid-2');
$events->fire('Illuminate\\Queue\\Events\\JobProcessed', $e);
$mail = new FakeMailEvent();
$mail->message = new \stdClass();
$events->fire('Illuminate\\Mail\\Events\\MessageSending', $mail);
$events->fire('*', 'App\\Domain\\OrderPlaced');
`)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want only the job: %v", len(lines), lines)
	}
	var e struct {
		Kind string `json:"kind"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("bad JSON line %q: %v", lines[0], err)
	}
	if e.Kind != "job" || e.Data.Status != "processed" {
		t.Errorf("got %s/%s, want job/processed", e.Kind, e.Data.Status)
	}
}

// TestLaravelAdapterPHP_QueuedInTheDispatchingRequest checks a job is reported
// where it was dispatched, so a request that queues work shows it even before
// any worker picks it up, carrying what the job holds and the uuid the worker
// will report the same job under.
func TestLaravelAdapterPHP_QueuedInTheDispatchingRequest(t *testing.T) {
	lines := runLaravelAdapterPHP(t, `<?php
namespace Illuminate\Bus { trait Queueable { public $connection; public $queue; public $delay; public $middleware = []; } }
namespace Illuminate\Foundation\Queue { trait Queueable { use \Illuminate\Bus\Queueable; } }
namespace App\Enums {
    enum ProgramStatus: string { case Published = 'Published'; }
    enum Priority: int { case High = 9; }
}
namespace App\Models {
    class Program {
        private $attributes; private $hidden;
        public function __construct(array $a, array $h = []) { $this->attributes = $a; $this->hidden = $h; }
        public function getAttributes() { return $this->attributes; }
        public function getHidden() { return $this->hidden; }
        public function getKey() { return $this->attributes['id'] ?? null; }
    }
}
namespace App\Jobs {
    class Base { protected $frameworkPlumbing = 'ignore me'; }
    class SendInvoice extends Base {
        use \Illuminate\Foundation\Queue\Queueable;
        public $orderId = 42;
        protected $recipient = 'a@b.test';
        private $lines = [1, 2, 3];
        public $program;
        public $status;
        public $priority;
    }
}
namespace {
`+fakeLaravelApp+`
define('LERD_DEVTOOLS_ON', true);
require ADAPTER;
$events = $GLOBALS['__lerd_fake_app']['events'];
$queued = new FakeQueueEvent();
$queued->job = new \App\Jobs\SendInvoice();
$queued->job->program = new \App\Models\Program(['id' => 17, 'name' => 'Spring 2026', 'secret_token' => 'nope'], ['secret_token']);
$queued->job->status = \App\Enums\ProgramStatus::Published;
$queued->job->priority = \App\Enums\Priority::High;
$queued->connectionName = 'redis';
$queued->queue = 'emails';
$queued->payload = json_encode(['uuid' => 'job-uuid-9', 'displayName' => 'App\Jobs\SendInvoice']);
$events->fire('Illuminate\\Queue\\Events\\JobQueued', $queued);
}
`)
	if len(lines) != 1 {
		t.Fatalf("got %d events, want 1: %v", len(lines), lines)
	}
	var e struct {
		Data struct {
			Class   string            `json:"class"`
			Status  string            `json:"status"`
			Queue   string            `json:"queue"`
			UUID    string            `json:"uuid"`
			Payload map[string]string `json:"payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("bad JSON line %q: %v", lines[0], err)
	}
	if e.Data.Class != "App\\Jobs\\SendInvoice" || e.Data.Status != "queued" || e.Data.Queue != "emails" {
		t.Errorf("got %q/%q/%q, want App\\Jobs\\SendInvoice/queued/emails", e.Data.Class, e.Data.Status, e.Data.Queue)
	}
	if e.Data.UUID != "job-uuid-9" {
		t.Errorf("uuid = %q, want the one decoded out of the queued payload", e.Data.UUID)
	}
	// Scalars survive, a collection is labelled, and neither the base class's
	// property nor the thirteen a queue trait contributes are the developer's
	// payload.
	// A model names the record it is, and its stored attributes come one level
	// down under dotted keys. What the model hides stays hidden.
	want := map[string]string{
		"orderId":      "42",
		"recipient":    `"a@b.test"`,
		"lines":        "array(3)",
		"program":      "Program #17",
		"program.id":   "17",
		"program.name": `"Spring 2026"`,
		"status":       "ProgramStatus::Published",
		"priority":     "Priority::High (9)",
	}
	if len(e.Data.Payload) != len(want) {
		t.Fatalf("payload = %v, want %v", e.Data.Payload, want)
	}
	for k, v := range want {
		if e.Data.Payload[k] != v {
			t.Errorf("payload[%s] = %q, want %q", k, e.Data.Payload[k], v)
		}
	}
}
