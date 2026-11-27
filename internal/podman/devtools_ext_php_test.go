package podman

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// runDevtoolsExt runs body with the compiled lerd_devtools capturing queries,
// returning the lines and connections it sent.
func runDevtoolsExt(t *testing.T, body string, dropEach bool, exts ...string) ([]string, int) {
	t.Helper()
	php, args, dir, sink := devtoolsExtSetup(t, body, dropEach, exts...)
	if out, err := exec.Command(php, append(args, filepath.Join(dir, "probe.php"))...).CombinedOutput(); err != nil {
		t.Fatalf("php run failed: %v\n%s", err, out)
	}
	return sink.result()
}

// devtoolsExtSetup writes body to dir/probe.php and returns the php arguments
// that load lerd_devtools against a capture socket. Skipped unless php can load
// the extension, which holds in lerd's FPM image and not on a CI runner.
func devtoolsExtSetup(t *testing.T, body string, dropEach bool, exts ...string) (string, []string, string, *capture) {
	t.Helper()
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	args := []string{"-n", "-d", "auto_prepend_file=", "-d", "extension=lerd_devtools"}
	for _, e := range exts {
		args = append(args, "-d", "extension="+e)
	}
	// Match the module line: a failed load prints a warning naming it too.
	if out, _ := exec.Command(php, append(args, "-m")...).Output(); !slices.Contains(strings.Split(string(out), "\n"), "lerd_devtools") {
		t.Skip("php cannot load lerd_devtools; run inside lerd's FPM image")
	}

	dir := t.TempDir()
	flag := filepath.Join(dir, "enabled.flag")
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// lerd's own php wrapper runs in a container that can't see this dir.
	if out, _ := exec.Command(php, "-n", "-r", "echo file_exists("+phpQuote(flag)+") ? 'Y' : 'N';").Output(); !strings.Contains(string(out), "Y") {
		t.Skip("php cannot read host files (containerised/sandboxed wrapper); native php needed")
	}
	if err := os.WriteFile(filepath.Join(dir, "probe.php"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sink := listenCapture(t, dropEach)
	args = append(args,
		"-d", "lerd.devtools_host=unix://"+sink.sock,
		"-d", "lerd.devtools_kinds=query",
		"-d", "lerd.devtools_flag="+flag,
		"-d", "lerd.assets_dir="+dir)
	return php, args, dir, sink
}

func queryLines(t *testing.T, lines []string) int {
	t.Helper()
	n := 0
	for _, l := range lines {
		var e struct{ Kind string }
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", l, err)
		}
		if e.Kind == "query" {
			n++
		}
	}
	return n
}

const extQueries = `<?php
$db = new PDO('sqlite::memory:');
for ($i = 0; $i < 5; $i++) {
    $db->query('select ' . $i);
    usleep(SLEEP);
}
`

func TestDevtoolsExt_OneConnectionPerRequest(t *testing.T) {
	lines, conns := runDevtoolsExt(t, strings.Replace(extQueries, "SLEEP", "0", 1), false)
	if got := queryLines(t, lines); got != 5 {
		t.Fatalf("query events = %d, want 5: %v", got, lines)
	}
	if conns != 1 {
		t.Errorf("connections = %d, want 1 for the whole request", conns)
	}
}

func TestDevtoolsExt_ReopensAClosedConnection(t *testing.T) {
	lines, conns := runDevtoolsExt(t, strings.Replace(extQueries, "SLEEP", "100000", 1), true)
	if got := queryLines(t, lines); got != 5 {
		t.Fatalf("query events = %d, want all 5 despite the socket closing after each: %v", got, lines)
	}
	if conns != 5 {
		t.Errorf("connections = %d, want 5", conns)
	}
}

// A forked child must open its own connection; sharing the parent's would
// interleave the two processes' lines.
func TestDevtoolsExt_ForkedChildOpensItsOwnConnection(t *testing.T) {
	lines, conns := runDevtoolsExt(t, `<?php
if (!function_exists('pcntl_fork')) { echo 'SKIP'; exit; }
$db = new PDO('sqlite::memory:');
$db->query('select 1');
$pid = pcntl_fork();
$db->query('select 2');
if ($pid === 0) { exit(0); }
pcntl_waitpid($pid, $status);
$db->query('select 3');
`, false, "pcntl")
	if got := queryLines(t, lines); got != 4 {
		t.Fatalf("query events = %d, want 4 (1 before the fork, 1 from each side, 1 after): %v", got, lines)
	}
	if conns != 2 {
		t.Errorf("connections = %d, want 2, the parent's and the child's", conns)
	}
}

// The CLI exits right after its one request, so this serves requests from a
// long-lived php -S instead. On macOS the connection is TCP, and one closed per
// request left a port in TIME_WAIT each time, so a worker keeps one.
func TestDevtoolsExt_KeepsConnectionAcrossRequests(t *testing.T) {
	php, args, dir, sink := devtoolsExtSetup(t, `<?php
(new PDO('sqlite::memory:'))->query('select 1');
echo 'ok';
`, false)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	srv := exec.Command(php, append(args, "-S", addr, "-t", dir)...)
	if err := srv.Start(); err != nil {
		t.Fatalf("php -S: %v", err)
	}
	t.Cleanup(func() { _ = srv.Process.Kill(); _ = srv.Wait() })

	for i := 0; i < 2; i++ {
		var resp *http.Response
		for try := 0; try < 50; try++ {
			if resp, err = http.Get("http://" + addr + "/probe.php"); err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if err != nil {
			t.Fatalf("GET probe %d: %v", i, err)
		}
		resp.Body.Close()
	}

	lines, conns := sink.result()
	sink.mu.Lock()
	ended := sink.ended
	sink.mu.Unlock()
	if queryLines(t, lines) != 2 || conns != 1 {
		t.Fatalf("got %d lines over %d connections, want 2 over 1: %v", len(lines), conns, lines)
	}
	if ended != 0 {
		t.Errorf("connection closed between requests, with php still running")
	}
}

// Every way PHP makes a request reaches the lens exactly once: curl easy and
// multi handles (Guzzle, Stripe, Symfony's CurlHttpClient) and the http stream
// wrapper (file_get_contents, fopen, Symfony's NativeHttpClient).
func TestDevtoolsExt_CapturesEveryHTTPClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Path", r.URL.Path)
		w.Write([]byte("hi " + r.URL.Path + " " + string(body)))
	}))
	defer srv.Close()
	php, args, dir, sink := devtoolsExtSetup(t, strings.ReplaceAll(`<?php
if (!function_exists('curl_init')) { echo 'NOCURL'; exit; }
echo defined('LERD_DEVTOOLS_HTTP') && LERD_DEVTOOLS_HTTP ? 'HTTP=1' : 'HTTP=0';
if (PHP_VERSION_ID < 80200) { echo ' SKIP'; exit; }
echo function_exists('http_get_last_response_headers') ? ' HEADERS=1' : '';
$ch = curl_init(SRV.'/easy');
curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
if (curl_exec($ch) !== 'hi /easy ') { echo ' CLIENT-LOST-BODY'; }

// Reused through curl_reset the way Guzzle does, with its own write callback.
$got = '';
$ch = curl_init();
curl_setopt_array($ch, [CURLOPT_URL => SRV.'/write', CURLOPT_POSTFIELDS => 'token=t1&a=1', CURLOPT_WRITEFUNCTION => function ($h, $d) use (&$got) { $got .= $d; return strlen($d); }]);
curl_exec($ch);
curl_reset($ch);
curl_setopt_array($ch, [CURLOPT_URL => SRV.'/reset', CURLOPT_RETURNTRANSFER => true]);
if (curl_exec($ch) !== 'hi /reset ' || $got !== 'hi /write token=t1&a=1') { echo ' CLIENT-LOST-BODY'; }

$mh = curl_multi_init();
$ch = curl_init(SRV.'/multi');
curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
curl_multi_add_handle($mh, $ch);
do { curl_multi_exec($mh, $running); curl_multi_select($mh); } while ($running);
while (curl_multi_info_read($mh)) {}

file_get_contents(SRV.'/get');
fclose(fopen(SRV.'/open', 'r'));
file_get_contents(__FILE__);
`, "SRV", phpQuote(srv.URL)), false)
	collector, err := DevtoolsCollectorPHP()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "devtools-collector.php"), []byte(collector), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(php, append(args, filepath.Join(dir, "probe.php"))...).CombinedOutput()
	if err != nil {
		t.Fatalf("php run failed: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "NOCURL") {
		t.Skip("php -n has no curl")
	}
	if strings.Contains(string(out), "SKIP") {
		if !strings.Contains(string(out), "HTTP=0") {
			t.Errorf("LERD_DEVTOOLS_HTTP set where the curl seam cannot fire, so Laravel's Http calls would vanish: %s", out)
		}
		t.Skip("observers only see built-in functions from PHP 8.2")
	}
	if strings.Contains(string(out), "CLIENT-LOST-BODY") {
		t.Errorf("a client stopped receiving its own response body: %s", out)
	}
	if !strings.Contains(string(out), "HTTP=1") {
		t.Errorf("LERD_DEVTOOLS_HTTP not set, so the Laravel adapter would report Http client calls twice: %s", out)
	}

	lines, _ := sink.result()
	var paths []string
	for _, l := range lines {
		var e struct {
			Kind string
			Data struct {
				URL      string
				Status   int
				Request  struct{ Body string }
				Response struct {
					Headers []string
					Body    string
				}
			}
		}
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("bad JSON line %q: %v", l, err)
		}
		if e.Kind != "http" {
			continue
		}
		// Before 8.4 file_get_contents leaves its headers where only the caller can read them.
		if e.Data.Status != 200 && !(strings.HasSuffix(e.Data.URL, "/get") && !strings.Contains(string(out), "HEADERS=1")) {
			t.Errorf("%s status = %d, want 200", e.Data.URL, e.Data.Status)
		}
		path := strings.TrimPrefix(e.Data.URL, srv.URL)
		paths = append(paths, path)
		if path == "/open" {
			continue // fopen hands back a stream, not a body
		}
		if want := "hi " + path + " " + map[string]string{"/write": "token=t1&a=1"}[path]; e.Data.Response.Body != want {
			t.Errorf("%s response body = %q, want %q", path, e.Data.Response.Body, want)
		}
		if path == "/write" && e.Data.Request.Body != "token=••••&a=1" {
			t.Errorf("/write request body = %q, want the token masked", e.Data.Request.Body)
		}
		if path == "/reset" && e.Data.Request.Body != "" {
			t.Errorf("/reset request body = %q, want nothing carried over from before curl_reset", e.Data.Request.Body)
		}
		if !slices.Contains(e.Data.Response.Headers, "X-Path: "+path) && path != "/get" {
			t.Errorf("%s response headers = %v, want X-Path", path, e.Data.Response.Headers)
		}
	}
	if want := []string{"/easy", "/write", "/reset", "/multi", "/get", "/open"}; !slices.Equal(paths, want) {
		t.Errorf("http events = %v, want %v once each", paths, want)
	}
}

// Symfony HttpClient keeps its own hook below PHP 8.2, where the curl seam
// cannot fire, and drops it from 8.2, where the seam reports the request.
func TestDevtoolsExt_SymfonyHookOnlyBelowPHP82(t *testing.T) {
	php, args, dir, sink := devtoolsExtSetup(t, `<?php
namespace Symfony\Component\HttpClient { class CurlHttpClient { function request($m, $u, $o = []) {} } }
namespace { echo PHP_VERSION_ID; (new Symfony\Component\HttpClient\CurlHttpClient())->request('GET', 'https://api.test/x'); }
`, false)
	collector, err := DevtoolsCollectorPHP()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "devtools-collector.php"), []byte(collector), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(php, append(args, filepath.Join(dir, "probe.php"))...).CombinedOutput()
	if err != nil {
		t.Fatalf("php run failed: %v\n%s", err, out)
	}
	lines, _ := sink.result()
	n := 0
	for _, l := range lines {
		if strings.Contains(l, `"kind":"http"`) {
			n++
		}
	}
	want := 0
	if v := strings.TrimSpace(string(out)); v < "80200" {
		want = 1
	}
	if n != want {
		t.Errorf("PHP %s: http events = %d, want %d", out, n, want)
	}
}
