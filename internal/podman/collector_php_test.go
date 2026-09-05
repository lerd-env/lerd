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
	return runCollectorPHPIn(t, t.TempDir(), body)
}

// runCollectorPHPIn is runCollectorPHP with the working directory supplied, for
// a test that has to put a seam file next to the collector before it runs.
func runCollectorPHPIn(t *testing.T, dir string, body string) []string {
	t.Helper()
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}

	collector, err := DevtoolsCollectorPHP()
	if err != nil {
		t.Fatalf("DevtoolsCollectorPHP: %v", err)
	}
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
	cmd.Env = append(os.Environ(),
		"LERD_DEVTOOLS_HOST=unix://"+sock,
		"LERD_DEVTOOLS_SEAMS="+filepath.Join(dir, "devtools-seams.conf"),
	)
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

// TestCollectorPHP_MessengerWorkerLifecycle checks that Messenger's worker
// events become job events with the status the worker reached, since the bus
// seam alone only ever says a message was dispatched.
func TestCollectorPHP_MessengerWorkerLifecycle(t *testing.T) {
	got := runCollectorPHP(t, `<?php
namespace Symfony\Component\Messenger {
    class Envelope {
        private $m; private $stamps;
        function __construct($m, array $stamps = []) { $this->m = $m; $this->stamps = $stamps; }
        function getMessage() { return $this->m; }
        function last($fqcn) { return $this->stamps[$fqcn] ?? null; }
    }
}
namespace Symfony\Component\Messenger\Stamp { class ReceivedStamp {} }
namespace Symfony\Component\Messenger\Event {
    class WorkerMessageReceivedEvent {
        protected $e; protected $r;
        function __construct($e, $r) { $this->e = $e; $this->r = $r; }
        function getEnvelope() { return $this->e; }
        function getReceiverName() { return $this->r; }
    }
    class WorkerMessageHandledEvent extends WorkerMessageReceivedEvent {}
    class WorkerMessageFailedEvent extends WorkerMessageReceivedEvent {
        private $t;
        function __construct($e, $r, $t) { parent::__construct($e, $r); $this->t = $t; }
        function getThrowable() { return $this->t; }
    }
}
namespace App\Message { class SendInvoice {} }
namespace {
    require COLLECTOR;
    $env = new \Symfony\Component\Messenger\Envelope(new \App\Message\SendInvoice());
    \Lerd\Collector\event(new \Symfony\Component\Messenger\Event\WorkerMessageReceivedEvent($env, 'async'), null);
    usleep(20000);
    \Lerd\Collector\event(new \Symfony\Component\Messenger\Event\WorkerMessageHandledEvent($env, 'async'), null);
    \Lerd\Collector\event(new \Symfony\Component\Messenger\Event\WorkerMessageFailedEvent($env, 'async', new \RuntimeException('smtp down')), null);
    // The worker hands the envelope it received back to the bus to run the
    // handler; that is not a new dispatch and must not be reported as one.
    $received = new \Symfony\Component\Messenger\Envelope(
        new \App\Message\SendInvoice(),
        ['Symfony\\Component\\Messenger\\Stamp\\ReceivedStamp' => new \Symfony\Component\Messenger\Stamp\ReceivedStamp()]
    );
    \Lerd\Collector\job($received);
}
`)

	type ev struct {
		Kind string `json:"kind"`
		Ctx  struct {
			RID string `json:"rid"`
		} `json:"ctx"`
		Data struct {
			Class     string  `json:"class"`
			Status    string  `json:"status"`
			Queue     string  `json:"queue"`
			TimeMS    float64 `json:"time_ms"`
			Exception string  `json:"exception"`
		} `json:"data"`
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want the three worker states only: %v", len(got), got)
	}
	var events []ev
	for _, line := range got {
		var e ev
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", line, err)
		}
		if e.Kind != "job" {
			t.Errorf("kind = %q, want job", e.Kind)
		}
		if e.Data.Class != "App\\Message\\SendInvoice" {
			t.Errorf("class = %q, want the message inside the envelope", e.Data.Class)
		}
		if e.Data.Queue != "async" {
			t.Errorf("queue = %q, want async", e.Data.Queue)
		}
		events = append(events, e)
	}
	for i, want := range []string{"processing", "processed", "failed"} {
		if events[i].Data.Status != want {
			t.Errorf("event %d status = %q, want %q", i, events[i].Data.Status, want)
		}
	}
	if events[1].Data.TimeMS <= 0 {
		t.Errorf("processed time_ms = %v, want the time the worker spent on it", events[1].Data.TimeMS)
	}
	if events[2].Data.Exception != "smtp down" {
		t.Errorf("failed exception = %q, want smtp down", events[2].Data.Exception)
	}
	if events[0].Ctx.RID == "" {
		t.Error("ctx.rid is empty, each message needs its own group")
	}
}

// TestCollectorPHP_WorkerReportsJobsOnly checks the capture policy a worker runs
// under when the user has not opted into full worker capture: its jobs are
// reported, everything else it dispatches is not.
func TestCollectorPHP_WorkerReportsJobsOnly(t *testing.T) {
	got := runCollectorPHP(t, `<?php
namespace Symfony\Component\Messenger {
    class Envelope {
        private $m;
        function __construct($m) { $this->m = $m; }
        function getMessage() { return $this->m; }
    }
}
namespace Symfony\Component\Messenger\Event {
    class WorkerMessageHandledEvent {
        private $e;
        function __construct($e) { $this->e = $e; }
        function getEnvelope() { return $this->e; }
    }
}
namespace App\Message { class SendInvoice {} }
namespace {
    define('LERD_DEVTOOLS_ON', false);
    define('LERD_DEVTOOLS_JOBS', true);
    require COLLECTOR;
    \Lerd\Collector\event(new \stdClass(), 'App\\Domain\\OrderPlaced');
    \Lerd\Collector\event(new \Symfony\Component\Messenger\Event\WorkerMessageHandledEvent(
        new \Symfony\Component\Messenger\Envelope(new \App\Message\SendInvoice())
    ), null);
}
`)
	if len(got) != 1 {
		t.Fatalf("got %d events, want only the job: %v", len(got), got)
	}
	var e struct {
		Kind string `json:"kind"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(got[0]), &e); err != nil {
		t.Fatalf("bad JSON line %q: %v", got[0], err)
	}
	if e.Kind != "job" || e.Data.Status != "processed" {
		t.Errorf("got %s/%s, want job/processed", e.Kind, e.Data.Status)
	}
}

// TestCollectorPHP_StoreSeamReportsAJob checks a store-declared seam turns one
// observed call into a job that starts and then finishes or fails, with the
// name resolved through the declared expression rather than hardcoded.
func TestCollectorPHP_StoreSeamReportsAJob(t *testing.T) {
	dir := t.TempDir()
	seams := "# header\n" +
		"job|implements|Fixture\\Queue\\JobInterface|process|this\n" +
		"job|class|Fixture_Action|execute|this.method:get_hook\n"
	if err := os.WriteFile(filepath.Join(dir, "devtools-seams.conf"), []byte(seams), 0o644); err != nil {
		t.Fatalf("write seams: %v", err)
	}
	got := runCollectorPHPIn(t, dir, `<?php
namespace Fixture\Queue { interface JobInterface { public function process(); } }
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
    trait Dispatchable { public $connection; public $delay; }
    class Base { protected $frameworkPlumbing = 'ignore me'; }
    class SendInvoice extends Base implements \Fixture\Queue\JobInterface {
        use Dispatchable;
        public $orderId = 42;
        protected $recipient = 'a@b.test';
        private $lines = [1, 2, 3];
        public $program;
        public $status;
        public $priority;
        public function process() {}
    }
}
namespace {
    class Fixture_Action { public function get_hook() { return 'scheduled_payment'; } public function execute() {} }
    require COLLECTOR;
    $job = new \App\Jobs\SendInvoice();
    $job->program = new \App\Models\Program(['id' => 17, 'name' => 'Spring 2026', 'secret_token' => 'nope'], ['secret_token']);
    $job->status = \App\Enums\ProgramStatus::Published;
    $job->priority = \App\Enums\Priority::High;
    \Lerd\Collector\seam_begin('App\\Jobs\\SendInvoice', 'process', $job, []);
    usleep(15000);
    \Lerd\Collector\seam_end('App\\Jobs\\SendInvoice', 'process', false);

    $action = new \Fixture_Action();
    \Lerd\Collector\seam_begin('Fixture_Action', 'execute', $action, []);
    \Lerd\Collector\seam_end('Fixture_Action', 'execute', true, 'the gateway refused');

    // A call no seam claims must not close somebody else's job.
    \Lerd\Collector\seam_begin('App\\Unrelated', 'process', new \stdClass(), []);
    \Lerd\Collector\seam_end('App\\Unrelated', 'process', false);
}
`)

	type ev struct {
		Kind string `json:"kind"`
		Ctx  struct {
			RID string `json:"rid"`
		} `json:"ctx"`
		Data struct {
			Class     string            `json:"class"`
			Status    string            `json:"status"`
			TimeMS    float64           `json:"time_ms"`
			Exception string            `json:"exception"`
			Payload   map[string]string `json:"payload"`
		} `json:"data"`
	}
	if len(got) != 4 {
		t.Fatalf("got %d events, want two per claimed seam and none for the unclaimed one: %v", len(got), got)
	}
	var events []ev
	for _, line := range got {
		var e ev
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", line, err)
		}
		if e.Kind != "job" {
			t.Errorf("kind = %q, want job", e.Kind)
		}
		events = append(events, e)
	}
	if events[0].Data.Class != "App\\Jobs\\SendInvoice" || events[0].Data.Status != "processing" {
		t.Errorf("first event = %q/%q, want the job class and processing", events[0].Data.Class, events[0].Data.Status)
	}
	if events[1].Data.Status != "processed" || events[1].Data.TimeMS <= 0 {
		t.Errorf("second event = %q/%v, want processed and a duration", events[1].Data.Status, events[1].Data.TimeMS)
	}
	// The accessor names the job by what it runs, not by the class running it.
	if events[2].Data.Class != "scheduled_payment" {
		t.Errorf("third event class = %q, want the resolved hook name", events[2].Data.Class)
	}
	if events[3].Data.Status != "failed" || events[3].Data.Exception != "the gateway refused" {
		t.Errorf("fourth event = %q/%q, want failed with the throwable's message", events[3].Data.Status, events[3].Data.Exception)
	}
	if events[0].Ctx.RID == events[2].Ctx.RID {
		t.Error("each job needs its own group, got one rid for both")
	}
	// What the job holds, at every state, with scalars kept and neither the
	// base class's property nor the trait's counted as the job's own.
	for _, i := range []int{0, 1} {
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
		if len(events[i].Data.Payload) != len(want) {
			t.Fatalf("event %d payload = %v, want %v", i, events[i].Data.Payload, want)
		}
		for k, v := range want {
			if events[i].Data.Payload[k] != v {
				t.Errorf("event %d payload[%s] = %q, want %q", i, k, events[i].Data.Payload[k], v)
			}
		}
	}
}
