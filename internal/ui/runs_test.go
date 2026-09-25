package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubRunExec replaces the command seam with a scripted one, so run behaviour
// is testable without spawning lerd.
func stubRunExec(t *testing.T, fn func(r *run) error) {
	t.Helper()
	original := execRun
	execRun = func(_ context.Context, r *run, _ []string, _ string) error { return fn(r) }
	t.Cleanup(func() { execRun = original })
}

func waitForStatus(t *testing.T, r *run, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r.snapshot().Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run status = %q, want %q", r.snapshot().Status, want)
}

// A run outlives the request that started it: its output is buffered so a page
// that reloads mid-composer can read back what it missed.
func TestRunReplaysBufferedOutput(t *testing.T) {
	stubRunExec(t, func(r *run) error {
		r.append("Installing dependencies")
		r.append("done")
		return nil
	})

	reg := newRunRegistry()
	r := reg.Start(runKindScaffold, t.TempDir(), "myapp", []string{"lerd", "new"})
	waitForStatus(t, r, runDone)

	lines, next, done := r.read(0)
	if len(lines) != 2 || lines[0] != "Installing dependencies" {
		t.Fatalf("replayed %v, want both lines from the start", lines)
	}
	if !done {
		t.Error("a finished run should read as done")
	}
	if again, _, _ := r.read(next); len(again) != 0 {
		t.Errorf("reading on from %d returned %v, want nothing", next, again)
	}
}

// A failed run keeps why it failed, so a reattaching page shows the reason
// rather than an empty success.
func TestRunRecordsFailure(t *testing.T) {
	stubRunExec(t, func(r *run) error {
		r.append("could not resolve dependencies")
		return context.DeadlineExceeded
	})

	reg := newRunRegistry()
	r := reg.Start(runKindScaffold, t.TempDir(), "", []string{"lerd", "new"})
	waitForStatus(t, r, runFailed)

	if snap := r.snapshot(); snap.Error == "" {
		t.Error("a failed run should carry its error")
	}
}

// Output is capped: a run that prints more than the buffer holds keeps the tail
// and tells a reader that fell behind where the buffer now starts.
func TestRunCapsBufferedOutput(t *testing.T) {
	stubRunExec(t, func(r *run) error {
		for i := 0; i < runMaxLines+50; i++ {
			r.append("line")
		}
		return nil
	})

	reg := newRunRegistry()
	r := reg.Start(runKindSetup, t.TempDir(), "", []string{"lerd", "setup"})
	waitForStatus(t, r, runDone)

	lines, _, _ := r.read(0)
	if len(lines) != runMaxLines {
		t.Fatalf("buffered %d lines, want the last %d", len(lines), runMaxLines)
	}
}

// A watcher blocks on the run rather than polling it, and wakes on the next
// line as well as on the end.
func TestRunWaitWakesOnOutput(t *testing.T) {
	r := &run{ID: "x", status: runRunning, Start: time.Now()}
	wait := r.wait()
	go r.append("hello")
	select {
	case <-wait:
	case <-time.After(time.Second):
		t.Fatal("waiter was not woken by a new line")
	}
}

// The wizard reattaches by directory, so runs are findable by the project they
// belong to, newest first.
func TestRunRegistryListsByDirectory(t *testing.T) {
	stubRunExec(t, func(_ *run) error { return nil })

	reg := newRunRegistry()
	mine := t.TempDir()
	other := t.TempDir()
	first := reg.Start(runKindLink, mine, "", []string{"lerd", "link"})
	waitForStatus(t, first, runDone)
	second := reg.Start(runKindSetup, mine, "", []string{"lerd", "setup"})
	waitForStatus(t, second, runDone)
	reg.Start(runKindLink, other, "", []string{"lerd", "link"})

	found := reg.ForDir(mine)
	if len(found) != 2 {
		t.Fatalf("found %d runs for the directory, want 2", len(found))
	}
	if found[0].ID != second.ID {
		t.Errorf("newest run = %q, want %q", found[0].ID, second.ID)
	}
}

// Every argv is built here from a named kind, so a page cannot ask lerd-ui to
// run something of its own choosing.
func TestRunArgvRejectsUnknownKind(t *testing.T) {
	_, _, _, err := runArgv("/usr/bin/lerd", RunRequest{Kind: "rm", Dir: t.TempDir()})
	if err == nil {
		t.Fatal("an unknown kind should be refused")
	}
}

func TestRunArgvBuildsScaffold(t *testing.T) {
	parent := t.TempDir()
	argv, dir, label, err := runArgv("/usr/bin/lerd", RunRequest{
		Kind: runKindScaffold, Dir: parent, Name: "myapp",
		Framework: "laravel", FrameworkVersion: "11",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dir != parent {
		t.Errorf("dir = %q, want the parent %q", dir, parent)
	}
	want := []string{"/usr/bin/lerd", "new", filepath.Join(parent, "myapp"), "--framework=laravel", "--framework-version=11"}
	if strings.Join(argv, " ") != strings.Join(want, " ") {
		t.Errorf("argv = %v, want %v", argv, want)
	}
	if label != filepath.Join(parent, "myapp") {
		t.Errorf("label = %q, want the target path", label)
	}
}

// A name is one directory under the chosen parent. Anything that climbs out of
// it, or that would be read as a flag, is refused before a command is built.
func TestRunArgvRejectsEscapingNames(t *testing.T) {
	parent := t.TempDir()
	for _, name := range []string{"", ".", "..", "../evil", "a/b", `a\b`, "--framework=x", "My App", "my app"} {
		if _, _, _, err := runArgv("/usr/bin/lerd", RunRequest{Kind: runKindScaffold, Dir: parent, Name: name}); err == nil {
			t.Errorf("name %q should be refused", name)
		}
	}
}

// Scaffolding into a directory that already exists would run composer over
// somebody's project, so it stops before the command starts.
func TestRunArgvRejectsExistingTarget(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, "taken"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := runArgv("/usr/bin/lerd", RunRequest{Kind: runKindScaffold, Dir: parent, Name: "taken"}); err == nil {
		t.Fatal("an existing target should be refused")
	}
}

// Setup steps are passed one flag at a time, so a label with spaces stays one
// step rather than being split into several.
func TestRunArgvBuildsSetupSteps(t *testing.T) {
	dir := t.TempDir()
	argv, _, _, err := runArgv("/usr/bin/lerd", RunRequest{
		Kind: runKindSetup, Dir: dir, Steps: []string{"composer install", "npm install/ci"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "/usr/bin/lerd setup --skip-open --step composer install --step npm install/ci"
	if strings.Join(argv, " ") != want {
		t.Errorf("argv = %v", argv)
	}
	if _, _, _, err := runArgv("/usr/bin/lerd", RunRequest{Kind: runKindSetup, Dir: dir}); err == nil {
		t.Error("a setup run naming no steps should be refused")
	}
}

// Starting a run reaches the host, so a remote session without the opt-in is
// turned away the way the editor and the other host actions turn it away.
func TestHandleRunsRequiresHostAuthority(t *testing.T) {
	body, _ := json.Marshal(RunRequest{Kind: runKindLink, Dir: t.TempDir()})
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader(body))
	req.RemoteAddr = "8.8.8.8:5000"
	rr := httptest.NewRecorder()
	handleRuns(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

// The stream replays what a run already printed and closes with how it ended,
// which is what a page reattaching after a reload reads.
func TestHandleRunStreamReplaysAndCloses(t *testing.T) {
	stubRunExec(t, func(r *run) error {
		r.append("composer install")
		return nil
	})
	r := runs.Start(runKindSetup, t.TempDir(), "", []string{"lerd", "setup"})
	waitForStatus(t, r, runDone)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+r.ID+"/stream", nil)
	rr := httptest.NewRecorder()
	handleRunStream(rr, req)

	out := rr.Body.String()
	if !strings.Contains(out, "data: composer install") {
		t.Errorf("stream did not replay the output: %q", out)
	}
	if !strings.Contains(out, "event: done") || !strings.Contains(out, `"ok":true`) {
		t.Errorf("stream did not close with the result: %q", out)
	}
}

// The stream is read by a client that does not unescape anything, so a line
// reaches the log exactly as it is written here. A PHP namespace is the case
// that matters: it is what a failed scaffold prints.
func TestHandleRunStreamWritesLinesVerbatim(t *testing.T) {
	stubRunExec(t, func(r *run) error {
		r.append(`Illuminate\Foundation\Bootstrap\HandleExceptions`)
		return nil
	})
	r := runs.Start(runKindSetup, t.TempDir(), "", []string{"lerd", "setup"})
	waitForStatus(t, r, runDone)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+r.ID+"/stream", nil)
	rr := httptest.NewRecorder()
	handleRunStream(rr, req)

	if out := rr.Body.String(); !strings.Contains(out, `data: Illuminate\Foundation\Bootstrap\HandleExceptions`) {
		t.Errorf("backslashes did not survive the stream unchanged: %q", out)
	}
}

// A carriage return inside a line would end the SSE data field early and split
// one line of output into a frame the client reads as another event.
func TestHandleRunStreamDropsEmbeddedCarriageReturns(t *testing.T) {
	stubRunExec(t, func(r *run) error {
		r.append("Downloading: 40%\rDownloading: 100%")
		return nil
	})
	r := runs.Start(runKindSetup, t.TempDir(), "", []string{"lerd", "setup"})
	waitForStatus(t, r, runDone)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+r.ID+"/stream", nil)
	rr := httptest.NewRecorder()
	handleRunStream(rr, req)

	out := rr.Body.String()
	if strings.Contains(out, "\r") {
		t.Errorf("a carriage return reached the stream and split the frame: %q", out)
	}
	if !strings.Contains(out, "data: Downloading: 40%Downloading: 100%") {
		t.Errorf("the line did not survive: %q", out)
	}
}

func TestHandleRunStreamUnknownRunIs404(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/runs/nope/stream", nil)
	rr := httptest.NewRecorder()
	handleRunStream(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

// Asking without a directory lists everything, which is how the dashboard knows
// something is still running before it decides whether to draw a spinner.
func TestHandleRunsListsEveryRunWithoutADirectory(t *testing.T) {
	stubRunExec(t, func(_ *run) error { return nil })
	started := runs.Start(runKindLink, t.TempDir(), "", []string{"lerd", "link"})
	waitForStatus(t, started, runDone)

	req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	rr := httptest.NewRecorder()
	handleRuns(rr, req)

	var out struct {
		Runs []runSnapshot `json:"runs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range out.Runs {
		if r.ID == started.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("the run is missing from the unfiltered listing: %+v", out.Runs)
	}
}

// Nothing else reads the pipe the command writes to, so a reader that gives up
// on an over-long line leaves the command blocked writing to it and the run
// running for ever. Composer and npm both write progress as one long line.
func TestExecRunSurvivesAnOverlongLine(t *testing.T) {
	r := &run{ID: "x", status: runRunning}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- execRun(ctx, r, []string{"sh", "-c", `head -c 2000000 /dev/zero | tr '\0' A; echo; echo tail`}, t.TempDir())
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("execRun: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("execRun never returned: the reader stopped draining the pipe")
	}

	lines, _, _ := r.read(0)
	if len(lines) == 0 || lines[len(lines)-1] != "tail" {
		t.Errorf("output after the long line was lost: %d lines", len(lines))
	}
}

// Retention has to mean what the constant says on an idle daemon too, not only
// on one that keeps being given new work.
func TestRunRegistryReleasesFinishedRunsWithoutANewRun(t *testing.T) {
	stubRunExec(t, func(r *run) error { return nil })
	dir := t.TempDir()
	r := runs.Start(runKindSetup, dir, "", []string{"lerd", "setup"})
	waitForStatus(t, r, runDone)

	r.mu.Lock()
	r.finished = time.Now().Add(-2 * runRetention)
	r.mu.Unlock()

	if got := runs.ForDir(dir); len(got) != 0 {
		t.Errorf("a run past its retention was still listed: %v", got)
	}
}
