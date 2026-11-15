package serviceops

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// A service is brought up once however many callers in this process ask for
// it together, and every one of them gets that run's answer.
func TestResume_sharesOneRunInAProcess(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	prev := resumeReady
	resumeReady = func(string) bool { return false }
	t.Cleanup(func() { resumeReady = prev })

	for _, want := range []error{nil, errors.New("mysql would not start")} {
		var runs atomic.Int32
		var wg sync.WaitGroup
		errs := make([]error, 8)
		for i := range errs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[i] = resume("mysql", func() error {
					runs.Add(1)
					time.Sleep(200 * time.Millisecond)
					return want
				})
			}()
		}
		wg.Wait()
		if got := runs.Load(); got != 1 {
			t.Errorf("brought up %d times, want once", got)
		}
		for i, err := range errs {
			if fmt.Sprint(err) != fmt.Sprint(want) {
				t.Errorf("caller %d got %v, want %v", i, err, want)
			}
		}
	}
}

// resumeHelper runs one resume in a process of its own, the way a lerd CLI
// call does, recording each bring-up in runs and its answer in result-<pid>.
func TestResumeHelperProcess(t *testing.T) {
	dir := os.Getenv("LERD_RESUME_DIR")
	if dir == "" {
		t.Skip("run by the cross-process tests")
	}
	ready := filepath.Join(dir, "ready")
	resumeReady = func(string) bool { _, err := os.Stat(ready); return err == nil }
	// An explicit start only gets the unit up; it never marks the service ready.
	active := filepath.Join(dir, "active")
	resumeActive = func(string) bool { _, err := os.Stat(active); return err == nil }
	run, marks := resume, ready
	if os.Getenv("LERD_RESUME_KIND") == "start" {
		run, marks = StartShared, active
	}
	err := run("mysql", func() error {
		f, _ := os.OpenFile(filepath.Join(dir, "runs"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		fmt.Fprintln(f, os.Getpid())
		f.Close()
		if os.Getenv("LERD_RESUME_HANG") != "" {
			time.Sleep(time.Minute)
		}
		time.Sleep(300 * time.Millisecond)
		if os.Getenv("LERD_RESUME_FAIL") != "" {
			return errors.New("mysql would not start")
		}
		return os.WriteFile(marks, nil, 0o644)
	})
	answer := "ok"
	if err != nil {
		answer = err.Error()
	}
	_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("result-%d", os.Getpid())), []byte(answer), 0o644)
}

func startHelper(t *testing.T, dir string, env ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestResumeHelperProcess$")
	cmd.Env = append(os.Environ(), append([]string{"LERD_RESUME_DIR=" + dir}, env...)...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Fields(string(b))
}

func answers(t *testing.T, dir string) []string {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(dir, "result-*"))
	var out []string
	for _, p := range paths {
		b, _ := os.ReadFile(p)
		out = append(out, string(b))
	}
	return out
}

// Separate lerd processes asking at once bring the service up once: the ones
// that waited find it ready, or get the error the run before them left.
func TestResume_sharesOneRunAcrossProcesses(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"ready", nil, "ok"},
		{"failed", []string{"LERD_RESUME_FAIL=1"}, "mysql would not start"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			var cmds []*exec.Cmd
			for range 5 {
				cmds = append(cmds, startHelper(t, dir, tc.env...))
			}
			for _, c := range cmds {
				_ = c.Wait()
			}
			if runs := lines(t, filepath.Join(dir, "runs")); len(runs) != 1 {
				t.Errorf("brought up %d times across processes, want once", len(runs))
			}
			got := answers(t, dir)
			if len(got) != 5 {
				t.Fatalf("%d of 5 processes answered: %v", len(got), got)
			}
			for _, a := range got {
				if a != tc.want {
					t.Errorf("a process answered %q, want %q", a, tc.want)
				}
			}
		})
	}
}

// A caller killed while it brings the service up leaves nothing held: the
// next one takes the lock and brings it up itself.
func TestResume_aKilledHolderDoesNotStrandTheNext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	holder := startHelper(t, dir, "LERD_RESUME_HANG=1")
	for deadline := time.Now().Add(10 * time.Second); len(lines(t, filepath.Join(dir, "runs"))) == 0; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the holder never started bringing the service up")
		}
	}
	next := startHelper(t, dir)
	time.Sleep(200 * time.Millisecond)
	_ = holder.Process.Signal(syscall.SIGKILL)
	_ = holder.Wait()

	done := make(chan error, 1)
	go func() { done <- next.Wait() }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		_ = next.Process.Kill()
		t.Fatal("the next caller was stranded behind a killed holder")
	}
	if runs := lines(t, filepath.Join(dir, "runs")); len(runs) != 2 {
		t.Errorf("brought up %d times, want the killed run and one more", len(runs))
	}
	if got := answers(t, dir); len(got) != 1 || got[0] != "ok" {
		t.Errorf("answers = %v, want the next caller's ok", got)
	}
}

// countingLifecycle starts a unit once it is asked to, counting the starts.
type countingLifecycle struct {
	noopLifecycle
	starts *atomic.Int32
	up     *atomic.Bool
}

func (l countingLifecycle) Start(string) error {
	l.starts.Add(1)
	time.Sleep(50 * time.Millisecond)
	l.up.Store(true)
	return nil
}

func (l countingLifecycle) UnitStatus(string) (string, error) {
	if l.up.Load() {
		return "active", nil
	}
	return "inactive", nil
}

// Every db:shell, php run and site link asks for its services; asked together
// for one that is down, it is started and probed once, not once per caller.
func TestEnsureServiceRunning_startsAndProbesOnceForCallersTogether(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	life := countingLifecycle{starts: &atomic.Int32{}, up: &atomic.Bool{}}
	podman.UnitLifecycle = life
	var probes atomic.Int32
	waitReadyFn = func(string, time.Duration) error {
		probes.Add(1)
		time.Sleep(150 * time.Millisecond)
		return nil
	}
	prevID, prevRegen := activationIDFn, regenConsumersFn
	activationIDFn = func(string) string { return "run-1" }
	// The starter's refresh of admin UIs probes on its own; not what is counted here.
	regenConsumersFn = func(string) {}
	t.Cleanup(func() { activationIDFn, regenConsumersFn = prevID, prevRegen })
	if err := config.SaveCustomService(&config.CustomService{
		Name: "proj-db", Image: "docker.io/library/postgres:16", Ports: []string{"127.0.0.1:5439:5432"},
	}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := EnsureServiceRunning("proj-db"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := life.starts.Load(); n != 1 {
		t.Errorf("started %d times, want once", n)
	}
	if n := probes.Load(); n != 1 {
		t.Errorf("probed %d times, want once", n)
	}
}

// An explicit start from the dashboard, the TUI or MCP and the starts a
// db:shell or a link make share one StartUnit when they meet.
func TestStartService_sharesTheStartWithEnsureCallers(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	life := countingLifecycle{starts: &atomic.Int32{}, up: &atomic.Bool{}}
	podman.UnitLifecycle = life
	var probes atomic.Int32
	waitReadyFn = func(string, time.Duration) error { probes.Add(1); time.Sleep(100 * time.Millisecond); return nil }
	prevID, prevRegen := activationIDFn, regenConsumersFn
	activationIDFn = func(string) string { return "run-1" }
	regenConsumersFn = func(string) {}
	t.Cleanup(func() { activationIDFn, regenConsumersFn = prevID, prevRegen })
	if err := config.SaveCustomService(&config.CustomService{
		Name: "proj-db", Image: "docker.io/library/postgres:16", Ports: []string{"127.0.0.1:5439:5432"},
	}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			call := EnsureServiceRunning
			if i%4 == 0 {
				call = StartService
			}
			if err := call("proj-db"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := life.starts.Load(); n != 1 {
		t.Errorf("started %d times, want once", n)
	}
	// The ensure callers were promised a ready service, so it was probed once,
	// never handed over as ready by a start that does not wait.
	if n := probes.Load(); n != 1 {
		t.Errorf("probed %d times, want once", n)
	}
}

// A name can come from a project's .env, so it never becomes part of a path:
// the lock and failure files stay in the lock folder whatever it holds.
func TestResume_aNameCannotLeaveTheLockFolder(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	prev := resumeReady
	resumeReady = func(string) bool { return false }
	t.Cleanup(func() { resumeReady = prev })

	for _, name := range []string{"../escape", "../../outside", "a/b"} {
		_ = resume(name, func() error { return errors.New("would not start") })
	}
	err := filepath.Walk(data, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Dir(path) != resumeLockDir() {
			t.Errorf("wrote %s outside the lock folder", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(data), "outside.lock")); err == nil {
		t.Error("a name climbed out of the data folder")
	}
}

// Explicit starts in separate processes, the dashboard's and a terminal's
// lerd service start, start the unit once: one that waited finds it up. On
// macOS a second start would re-create the container under the first.
func TestStartShared_startsOnceAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var cmds []*exec.Cmd
	for range 4 {
		cmds = append(cmds, startHelper(t, dir, "LERD_RESUME_KIND=start"))
	}
	for _, c := range cmds {
		_ = c.Wait()
	}
	if runs := lines(t, filepath.Join(dir, "runs")); len(runs) != 1 {
		t.Errorf("started %d times across processes, want once", len(runs))
	}
}
