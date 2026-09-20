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
	if out, _ := exec.Command(php, noBridge(preflight)...).CombinedOutput(); !strings.Contains(string(out), "Y") {
		t.Skip("php cannot read host files (containerised/sandboxed wrapper); native php needed")
	}

	sock := shortSocketPath(t)
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

	cmd := exec.Command(php, noBridge(scriptPath)...)
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

// noBridge runs a script with the host's own lerd instrumentation kept out of
// it. The harness loads its own copy of the collector on purpose, and on a
// machine running lerd the php.ini loads the engine collector into every
// process and auto-prepends the debug bridge, so without this the two copies
// collide on Lerd\Collector\host() and the host's captures arrive on the
// socket beside the ones under test. Not a product problem: nothing else
// includes the collector by hand.
func noBridge(script string) []string {
	return []string{"-n", "-d", "auto_prepend_file=", script}
}

// shortSocketPath returns a socket path under the macOS 104-byte sun_path
// limit. t.TempDir() embeds the test name, and these names are long enough that
// the socket fails to bind with "invalid argument", which reads as a broken
// collector rather than a path that is simply too long.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "lerdc")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "c.sock")
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

// TestCollectorPHP_RayCapturesLandAsDumps checks a store-declared ray capture
// turns the call the package would have shipped to the Ray app into a dump: a
// plain ray() labelled as one, a payload that built itself labelled by what it
// is, and the payloads that only tell the app how to draw itself dropped.
func TestCollectorPHP_RayCapturesLandAsDumps(t *testing.T) {
	dir := t.TempDir()
	seams := "# header\n" +
		"ray|class|Fixture\\Ray\\Ray|sendRequest|\n"
	if err := os.WriteFile(filepath.Join(dir, "devtools-seams.conf"), []byte(seams), 0o644); err != nil {
		t.Fatalf("write seams: %v", err)
	}
	got := runCollectorPHPIn(t, dir, `<?php
namespace Fixture\Ray {
    class Ray { public function sendRequest($payloads) {} }
    class Payload {
        private $type; private $content;
        public function __construct($type, $content) { $this->type = $type; $this->content = $content; }
        public function getType() { return $this->type; }
        public function getContent() { return $this->content; }
    }
}
namespace {
    require COLLECTOR;
    $ray = new \Fixture\Ray\Ray();

    // ray('hello') — the package converted the argument on its way out.
    $log = new \Fixture\Ray\Payload('log', ['values' => ['hello'], 'meta' => [['clipboard_data' => 'hello']]]);
    \Lerd\Collector\seam_begin('Fixture\\Ray\\Ray', 'sendRequest', $ray, [1 => $log]);
    \Lerd\Collector\seam_end('Fixture\\Ray\\Ray', 'sendRequest', false);

    // ray()->table([...]) — markup and all.
    $table = new \Fixture\Ray\Payload('table', [
        'values' => ['Name' => 'Ada', 'Rows' => '<pre class=sf-dump id=sf-dump-1>array:1 [&hellip;]</pre><script>sfdump()</script>'],
        'label' => 'Users',
    ]);
    \Lerd\Collector\seam_begin('Fixture\\Ray\\Ray', 'sendRequest', $ray, [1 => [$table]]);
    \Lerd\Collector\seam_end('Fixture\\Ray\\Ray', 'sendRequest', false);

    // ray()->green() — nothing to show in a window that is not Ray.
    $color = new \Fixture\Ray\Payload('color', ['color' => 'green']);
    \Lerd\Collector\seam_begin('Fixture\\Ray\\Ray', 'sendRequest', $ray, [1 => $color]);
    \Lerd\Collector\seam_end('Fixture\\Ray\\Ray', 'sendRequest', false);
}
`)

	type ev struct {
		Kind  string `json:"kind"`
		Label string `json:"label"`
		Text  string `json:"text"`
	}
	var events []ev
	for _, line := range got {
		var e ev
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", line, err)
		}
		events = append(events, e)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want the log and the table: %v", len(events), got)
	}
	for _, e := range events {
		if e.Kind != "dump" {
			t.Errorf("kind = %q, want dump", e.Kind)
		}
	}
	if events[0].Label != "ray" || events[0].Text != "hello" {
		t.Errorf("first event = %q/%q, want a plain ray and its value", events[0].Label, events[0].Text)
	}
	e := events[1]
	if e.Label != "ray:table" {
		t.Errorf("label = %q, want the payload type", e.Label)
	}
	if !strings.Contains(e.Text, "Name: Ada") || !strings.Contains(e.Text, "label: Users") {
		t.Errorf("table text = %q, want the payload's own values", e.Text)
	}
	if strings.Contains(e.Text, "<pre") || strings.Contains(e.Text, "sfdump()") {
		t.Errorf("table text = %q, want the markup taken back out of it", e.Text)
	}
	if !strings.Contains(e.Text, "array:1 […]") {
		t.Errorf("table text = %q, want the dump the markup was drawing", e.Text)
	}
}

// TestCollectorPHP_LogRecordsLandAsLogs checks a store-declared log capture
// turns a write to a logger into a log event: the channel from the logger, the
// level named whichever scale it arrived on, and the context rendered the way a
// dump is.
func TestCollectorPHP_LogRecordsLandAsLogs(t *testing.T) {
	dir := t.TempDir()
	seams := "# header\n" +
		"log|class|Fixture\\Log\\Logger|addRecord|\n"
	if err := os.WriteFile(filepath.Join(dir, "devtools-seams.conf"), []byte(seams), 0o644); err != nil {
		t.Fatalf("write seams: %v", err)
	}
	got := runCollectorPHPIn(t, dir, `<?php
namespace Fixture\Log {
    class Logger {
        private $name;
        public function __construct($name) { $this->name = $name; }
        public function getName() { return $this->name; }
        public function addRecord($level, $message, array $context = []) {}
    }
    // Monolog 3 hands its own enum, which names itself.
    class Level { public $name = 'Error'; public function getName() { return $this->name; } }
}
namespace {
    require COLLECTOR;
    $log = new \Fixture\Log\Logger('app');

    \Lerd\Collector\seam_begin('Fixture\\Log\\Logger', 'addRecord', $log, [1 => 400, 2 => 'the gateway refused', 3 => ['order' => 42]]);
    \Lerd\Collector\seam_end('Fixture\\Log\\Logger', 'addRecord', false);

    \Lerd\Collector\seam_begin('Fixture\\Log\\Logger', 'addRecord', $log, [1 => new \Fixture\Log\Level(), 2 => 'an enum level']);
    \Lerd\Collector\seam_end('Fixture\\Log\\Logger', 'addRecord', false);

    // An RFC 5424 severity, which either Monolog major accepts.
    \Lerd\Collector\seam_begin('Fixture\\Log\\Logger', 'addRecord', $log, [1 => 7, 2 => 'a debug line']);
    \Lerd\Collector\seam_end('Fixture\\Log\\Logger', 'addRecord', false);

    // Nothing to report without a message.
    \Lerd\Collector\seam_begin('Fixture\\Log\\Logger', 'addRecord', $log, [1 => 400]);
    \Lerd\Collector\seam_end('Fixture\\Log\\Logger', 'addRecord', false);
}
`)

	type ev struct {
		Kind string `json:"kind"`
		Data struct {
			Level   string `json:"level"`
			Channel string `json:"channel"`
			Message string `json:"message"`
			Context string `json:"context"`
		} `json:"data"`
	}
	var events []ev
	for _, line := range got {
		var e ev
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", line, err)
		}
		events = append(events, e)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want one per record with a message: %v", len(events), got)
	}
	for _, e := range events {
		if e.Kind != "log" {
			t.Errorf("kind = %q, want log", e.Kind)
		}
		if e.Data.Channel != "app" {
			t.Errorf("channel = %q, want the logger's own name", e.Data.Channel)
		}
	}
	if events[0].Data.Level != "error" || events[0].Data.Message != "the gateway refused" {
		t.Errorf("first record = %q/%q, want error and its message", events[0].Data.Level, events[0].Data.Message)
	}
	// Rendered by the cloner where the project has it and by print_r where it
	// does not, so the assertion is on the values rather than on the shape.
	if !strings.Contains(events[0].Data.Context, "order") || !strings.Contains(events[0].Data.Context, "42") {
		t.Errorf("context = %q, want the attached values rendered", events[0].Data.Context)
	}
	if events[1].Data.Level != "error" {
		t.Errorf("enum level = %q, want error", events[1].Data.Level)
	}
	if events[2].Data.Level != "debug" {
		t.Errorf("RFC severity = %q, want debug", events[2].Data.Level)
	}
}

// TestCollectorPHP_SentryEventsLandAsExceptions checks a store-declared
// exception capture reports what an app was about to send to Sentry: a
// throwable handed over in the hint, an event carrying its own exception, and
// a captured message, each with the level the event was raised at.
func TestCollectorPHP_SentryEventsLandAsExceptions(t *testing.T) {
	dir := t.TempDir()
	seams := "# header\n" +
		"exception|class|Fixture\\Sentry\\Client|captureEvent|sentry\n"
	if err := os.WriteFile(filepath.Join(dir, "devtools-seams.conf"), []byte(seams), 0o644); err != nil {
		t.Fatalf("write seams: %v", err)
	}
	got := runCollectorPHPIn(t, dir, `<?php
namespace Fixture\Sentry {
    class Client { public function captureEvent($event, $hint = null) {} }
    class Severity { private $n; public function __construct($n) { $this->n = $n; } public function __toString() { return $this->n; } }
    class Event {
        private $level; private $message; private $exceptions = [];
        public function __construct($level = null, $message = null, array $exceptions = []) {
            $this->level = $level; $this->message = $message; $this->exceptions = $exceptions;
        }
        public function getLevel() { return $this->level; }
        public function getMessage() { return $this->message; }
        public function getExceptions() { return $this->exceptions; }
    }
    class Bag {
        private $type; private $value;
        public function __construct($type, $value) { $this->type = $type; $this->value = $value; }
        public function getType() { return $this->type; }
        public function getValue() { return $this->value; }
    }
    class Hint { public $exception = null; }
}
namespace App\Billing {
    function charge() { throw new \RuntimeException('the gateway refused'); }
}
namespace {
    require COLLECTOR;
    $client = new \Fixture\Sentry\Client();

    // captureException: an empty event and the throwable on the hint.
    try {
        \App\Billing\charge();
    } catch (\RuntimeException $e) {
        $hint = new \Fixture\Sentry\Hint();
        $hint->exception = $e;
        \Lerd\Collector\seam_begin('Fixture\\Sentry\\Client', 'captureEvent', $client, [1 => new \Fixture\Sentry\Event(new \Fixture\Sentry\Severity('error')), 2 => $hint]);
        \Lerd\Collector\seam_end('Fixture\\Sentry\\Client', 'captureEvent', false);
    }

    // An event the app assembled itself, exception already attached.
    $event = new \Fixture\Sentry\Event(new \Fixture\Sentry\Severity('warning'), null, [new \Fixture\Sentry\Bag('App\\Exceptions\\Retryable', 'try again')]);
    \Lerd\Collector\seam_begin('Fixture\\Sentry\\Client', 'captureEvent', $client, [1 => $event]);
    \Lerd\Collector\seam_end('Fixture\\Sentry\\Client', 'captureEvent', false);

    // captureMessage.
    \Lerd\Collector\seam_begin('Fixture\\Sentry\\Client', 'captureEvent', $client, [1 => new \Fixture\Sentry\Event(new \Fixture\Sentry\Severity('info'), 'a note from the app')]);
    \Lerd\Collector\seam_end('Fixture\\Sentry\\Client', 'captureEvent', false);

    // An event with nothing in it is not a report.
    \Lerd\Collector\seam_begin('Fixture\\Sentry\\Client', 'captureEvent', $client, [1 => new \Fixture\Sentry\Event()]);
    \Lerd\Collector\seam_end('Fixture\\Sentry\\Client', 'captureEvent', false);
}
`)

	type frame struct {
		File string `json:"file"`
		Line int    `json:"line"`
	}
	type ev struct {
		Kind string `json:"kind"`
		Src  frame  `json:"src"`
		Data struct {
			Type    string  `json:"type"`
			Message string  `json:"message"`
			Level   string  `json:"level"`
			Source  string  `json:"source"`
			Trace   []frame `json:"trace"`
		} `json:"data"`
	}
	var events []ev
	for _, line := range got {
		var e ev
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", line, err)
		}
		events = append(events, e)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want one per report with something in it: %v", len(events), got)
	}
	for _, e := range events {
		if e.Kind != "exception" {
			t.Errorf("kind = %q, want exception", e.Kind)
		}
	}
	if events[0].Data.Type != "RuntimeException" || events[0].Data.Message != "the gateway refused" {
		t.Errorf("first event = %q/%q, want the thrown class and its message", events[0].Data.Type, events[0].Data.Message)
	}
	if events[0].Data.Level != "error" {
		t.Errorf("level = %q, want error", events[0].Data.Level)
	}
	// The frames are the throwable's own, so the first one is the throw site
	// rather than the line that handed the exception to Sentry.
	if len(events[0].Data.Trace) == 0 || !strings.HasSuffix(events[0].Src.File, "probe.php") {
		t.Errorf("src/trace = %+v / %d frames, want the throwable's own origin", events[0].Src, len(events[0].Data.Trace))
	}
	if events[1].Data.Type != "App\\Exceptions\\Retryable" || events[1].Data.Level != "warning" {
		t.Errorf("second event = %q/%q, want the attached exception and its level", events[1].Data.Type, events[1].Data.Level)
	}
	if events[2].Data.Type != "message" || events[2].Data.Message != "a note from the app" {
		t.Errorf("third event = %q/%q, want the captured message", events[2].Data.Type, events[2].Data.Message)
	}
	for i, e := range events {
		if e.Data.Source != "sentry" {
			t.Errorf("event %d source = %q, want the reporter the store named", i, e.Data.Source)
		}
	}
}

// TestCollectorPHP_ThrowableReportLandsAsException checks a reporter that hands
// over the throwable itself, the way Inspector does, is reported with the
// frames it was thrown from rather than the line that reported it.
func TestCollectorPHP_ThrowableReportLandsAsException(t *testing.T) {
	dir := t.TempDir()
	seams := "# header\n" +
		"exception|class|Fixture\\Apm\\Inspector|reportException|inspector\n"
	if err := os.WriteFile(filepath.Join(dir, "devtools-seams.conf"), []byte(seams), 0o644); err != nil {
		t.Fatalf("write seams: %v", err)
	}
	got := runCollectorPHPIn(t, dir, `<?php
namespace Fixture\Apm {
    class Inspector { public function reportException($throwable, $handled = true) {} }
}
namespace App\Billing {
    function charge() { throw new \RuntimeException('the gateway refused', 7); }
}
namespace {
    require COLLECTOR;
    $apm = new \Fixture\Apm\Inspector();
    try {
        \App\Billing\charge();
    } catch (\RuntimeException $e) {
        $wrapped = new \LogicException('checkout failed', 0, $e);
        \Lerd\Collector\seam_begin('Fixture\\Apm\\Inspector', 'reportException', $apm, [1 => $wrapped, 2 => true]);
        \Lerd\Collector\seam_end('Fixture\\Apm\\Inspector', 'reportException', false);
    }
}
`)

	type ev struct {
		Kind string `json:"kind"`
		Src  struct {
			File string `json:"file"`
			Line int    `json:"line"`
		} `json:"src"`
		Data struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			Level    string `json:"level"`
			Previous string `json:"previous"`
			Source   string `json:"source"`
		} `json:"data"`
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want the reported throwable: %v", len(got), got)
	}
	var e ev
	if err := json.Unmarshal([]byte(got[0]), &e); err != nil {
		t.Fatalf("bad JSON line %q: %v", got[0], err)
	}
	if e.Kind != "exception" || e.Data.Type != "LogicException" || e.Data.Message != "checkout failed" {
		t.Errorf("event = %q/%q/%q, want the reported class and its message", e.Kind, e.Data.Type, e.Data.Message)
	}
	if !strings.Contains(e.Data.Previous, "RuntimeException") || !strings.Contains(e.Data.Previous, "the gateway refused") {
		t.Errorf("previous = %q, want the cause it wrapped", e.Data.Previous)
	}
	if !strings.HasSuffix(e.Src.File, "probe.php") || e.Src.Line == 0 {
		t.Errorf("src = %+v, want the line it was thrown from", e.Src)
	}
	if e.Data.Source != "inspector" {
		t.Errorf("source = %q, want the reporter the store named", e.Data.Source)
	}
}

// TestCollectorPHP_NotifierMessagesLandAsMessages checks a store-declared
// message capture reports what a site sent to somebody: the channel it went
// on, the transport that carried it, who it went to and what it said.
func TestCollectorPHP_NotifierMessagesLandAsMessages(t *testing.T) {
	dir := t.TempDir()
	seams := "# header\n" +
		"message|class|Fixture\\Notifier\\Texter|send|notifier\n" +
		"message|class|Fixture\\Notifier\\Chatter|send|notifier\n"
	if err := os.WriteFile(filepath.Join(dir, "devtools-seams.conf"), []byte(seams), 0o644); err != nil {
		t.Fatalf("write seams: %v", err)
	}
	got := runCollectorPHPIn(t, dir, `<?php
namespace Fixture\Notifier {
    class Texter { public function send($message) {} }
    class Chatter { public function send($message) {} }
    class SmsMessage {
        public function __construct(private $phone, private $subject, private $from, private $transport) {}
        public function getPhone() { return $this->phone; }
        public function getRecipientId() { return $this->phone; }
        public function getSubject() { return $this->subject; }
        public function getFrom() { return $this->from; }
        public function getTransport() { return $this->transport; }
    }
    class ChatMessage {
        public function __construct(private $subject, private $transport) {}
        public function getSubject() { return $this->subject; }
        public function getRecipientId() { return null; }
        public function getTransport() { return $this->transport; }
    }
}
namespace {
    require COLLECTOR;
    $texter = new \Fixture\Notifier\Texter();
    $chatter = new \Fixture\Notifier\Chatter();

    $sms = new \Fixture\Notifier\SmsMessage('+40711000000', 'your order shipped', 'Acme', 'twilio');
    \Lerd\Collector\seam_begin('Fixture\\Notifier\\Texter', 'send', $texter, [1 => $sms]);
    \Lerd\Collector\seam_end('Fixture\\Notifier\\Texter', 'send', false);

    $chat = new \Fixture\Notifier\ChatMessage('deploy finished', 'slack');
    \Lerd\Collector\seam_begin('Fixture\\Notifier\\Chatter', 'send', $chatter, [1 => $chat]);
    \Lerd\Collector\seam_end('Fixture\\Notifier\\Chatter', 'send', false);

    // Something that is not a message must not be reported as one.
    \Lerd\Collector\seam_begin('Fixture\\Notifier\\Texter', 'send', $texter, [1 => new \stdClass()]);
    \Lerd\Collector\seam_end('Fixture\\Notifier\\Texter', 'send', false);
}
`)

	type ev struct {
		Kind string `json:"kind"`
		Data struct {
			Channel   string `json:"channel"`
			Transport string `json:"transport"`
			To        string `json:"to"`
			From      string `json:"from"`
			Body      string `json:"body"`
		} `json:"data"`
	}
	var events []ev
	for _, line := range got {
		var e ev
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", line, err)
		}
		events = append(events, e)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want one per message: %v", len(events), got)
	}
	for _, e := range events {
		if e.Kind != "message" {
			t.Errorf("kind = %q, want message", e.Kind)
		}
	}
	sms := events[0].Data
	if sms.Channel != "sms" || sms.Transport != "twilio" || sms.To != "+40711000000" || sms.From != "Acme" {
		t.Errorf("sms = %+v, want the channel, transport and both ends", sms)
	}
	if sms.Body != "your order shipped" {
		t.Errorf("body = %q, want what the message said", sms.Body)
	}
	chat := events[1].Data
	if chat.Channel != "chat" || chat.Transport != "slack" || chat.Body != "deploy finished" {
		t.Errorf("chat = %+v, want the chat channel and its transport", chat)
	}
}
