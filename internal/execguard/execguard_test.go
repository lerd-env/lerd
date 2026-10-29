package execguard

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func stubDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := dirFn
	dirFn = func() string { return dir }
	t.Cleanup(func() { dirFn = prev })
	return dir
}

func stubSignals(t *testing.T) chan os.Signal {
	t.Helper()
	sigs := make(chan os.Signal, 1)
	prev := notifyFn
	notifyFn = func() (<-chan os.Signal, func()) { return sigs, func() {} }
	t.Cleanup(func() { notifyFn = prev })
	return sigs
}

// stubKill records each kill and ends cmd, which stands in for podman exec the
// way the container's processes dying ends the real one.
func stubKill(t *testing.T, cmd *exec.Cmd) *[]string {
	t.Helper()
	var killed []string
	prev := killFn
	killFn = func(container string, markers ...string) {
		killed = append(killed, container+" "+strings.Join(markers, ","))
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	t.Cleanup(func() { killFn = prev })
	return &killed
}

// sleeper stands in for a long podman exec: `sh exec` runs a script named exec,
// so the argv starts the way Run looks for and the marker lands in its args.
func sleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "exec"), []byte("sleep 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "exec")
	cmd.Dir = dir
	return cmd
}

// markerOf reads the marker Run stamped into a podman exec argv.
func markerOf(t *testing.T, args []string) string {
	t.Helper()
	i := slices.Index(args, "--env")
	if i < 0 || !strings.HasPrefix(args[i+1], MarkerEnv+"=") {
		t.Fatalf("no marker in %v", args)
	}
	return strings.TrimPrefix(args[i+1], MarkerEnv+"=")
}

// The marker goes in as an exec flag, before the container name, so every
// process the run starts inherits it.
func TestRun_stampsTheExecWithItsOwnMarker(t *testing.T) {
	stubDir(t)
	cmd := exec.Command("true", "exec", "-i", "lerd-php85-fpm", "php", "-v")
	if err := Run(context.Background(), cmd, "lerd-php85-fpm"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cmd.Args[1:4], []string{"exec", "--env", MarkerEnv + "=" + markerOf(t, cmd.Args)}) {
		t.Fatalf("args = %v, want the marker right after exec", cmd.Args)
	}
}

// A run that ends on its own can still leave a child behind, one it put in the
// background or one Ctrl+C in a terminal did not reach, so its record stays for
// the sweep, marked ended.
func TestRun_marksItsRecordEndedWhenTheCommandEnds(t *testing.T) {
	dir := stubDir(t)
	if err := Run(context.Background(), exec.Command("true", "exec", "c"), "c"); err != nil {
		t.Fatal(err)
	}
	recs := readRecords(t, dir)
	if len(recs) != 1 || !recs[0].Ended {
		t.Fatalf("records = %+v, want one marked ended", recs)
	}
}

func readRecords(t *testing.T, dir string) []record {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var out []record
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var r record
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// Interrupting lerd kills what the run started in the container; podman exec
// alone would leave it running.
func TestRun_killsTheContainerSideWhenInterrupted(t *testing.T) {
	stubDir(t)
	sigs := stubSignals(t)
	var raised []os.Signal
	prev := raiseFn
	raiseFn = func(s os.Signal) { raised = append(raised, s) }
	t.Cleanup(func() { raiseFn = prev })
	cmd := sleeper(t)
	killed := stubKill(t, cmd)
	sigs <- syscall.SIGTERM

	_ = Run(context.Background(), cmd, "lerd-php85-fpm")
	if len(*killed) != 1 || (*killed)[0] != "lerd-php85-fpm "+markerOf(t, cmd.Args) {
		t.Fatalf("killed %v, want this run's processes", *killed)
	}
	// lerd mcp must still exit on the SIGTERM its session sent it, with the
	// record marked ended first, since the signal ends lerd before any defer.
	if len(raised) != 1 || raised[0] != syscall.SIGTERM {
		t.Fatalf("raised %v, want the signal passed back to lerd", raised)
	}
	if recs := readRecords(t, dirFn()); len(recs) != 1 || !recs[0].Ended {
		t.Fatalf("records = %+v, want one marked ended for the sweep", recs)
	}
}

// The re-raised signal ends lerd while Run returns, so a second record write
// in a defer gets cut short and strands an empty temp file for good.
func TestRun_writesNothingAfterPassingTheSignalBack(t *testing.T) {
	dir := stubDir(t)
	sigs := stubSignals(t)
	prev := raiseFn
	raiseFn = func(os.Signal) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	t.Cleanup(func() { raiseFn = prev })
	cmd := sleeper(t)
	stubKill(t, cmd)
	sigs <- syscall.SIGINT

	_ = Run(context.Background(), cmd, "lerd-php85-fpm")
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("wrote %v after the signal was passed back", entries)
	}
}

// The dashboard and MCP give up on a run through its context; the container
// side goes with it, and only this run's processes, not the others there.
func TestRun_killsTheContainerSideWhenItsContextEnds(t *testing.T) {
	stubDir(t)
	stubSignals(t)
	cmd := sleeper(t)
	killed := stubKill(t, cmd)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = Run(ctx, cmd, "lerd-php85-fpm")
	if len(*killed) != 1 || (*killed)[0] != "lerd-php85-fpm "+markerOf(t, cmd.Args) {
		t.Fatalf("killed %v, want this run's processes", *killed)
	}
}

func stubKillWait(t *testing.T, attempts int) {
	t.Helper()
	prevA, prevW := killAttempts, killWait
	killAttempts, killWait = attempts, 10*time.Millisecond
	t.Cleanup(func() { killAttempts, killWait = prevA, prevW })
}

// A kill that lands before the command has started inside the container finds
// nothing to kill, so it is tried again.
func TestRun_retriesAKillThatFoundNothingYet(t *testing.T) {
	stubDir(t)
	stubSignals(t)
	stubKillWait(t, 3)
	cmd := sleeper(t)
	calls := 0
	prev := killFn
	killFn = func(string, ...string) {
		calls++
		if calls == 2 {
			_ = cmd.Process.Kill()
		}
	}
	t.Cleanup(func() { killFn = prev })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = Run(ctx, cmd, "lerd-php85-fpm")
	if calls != 2 {
		t.Fatalf("kill tried %d times, want it repeated until the run ended", calls)
	}
}

// A run whose container side never answers must not hang lerd: the local
// podman client is ended, and the record stays for the sweep.
func TestRun_endsTheClientWhenTheContainerNeverAnswers(t *testing.T) {
	dir := stubDir(t)
	stubSignals(t)
	stubKillWait(t, 2)
	calls := 0
	prev := killFn
	killFn = func(string, ...string) { calls++ }
	t.Cleanup(func() { killFn = prev })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = Run(ctx, sleeper(t), "lerd-php85-fpm")
	if calls != 2 {
		t.Fatalf("kill tried %d times, want 2", calls)
	}
	if recs := readRecords(t, dir); len(recs) != 1 || !recs[0].Ended {
		t.Fatalf("records = %+v, want one marked ended", recs)
	}
}

// A record still being written is a .tmp file, which the sweep never reads.
func TestSweep_leavesARecordBeingWrittenAlone(t *testing.T) {
	dir := stubDir(t)
	stubKill(t, nil)
	tmp := filepath.Join(dir, "half.json.tmp")
	if err := os.WriteFile(tmp, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	Sweep()
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("sweep touched a record being written: %v", err)
	}
}

// Host commands, such as a native runtime's php, have no container side.
func TestRun_leavesACommandThatIsNotAnExecAlone(t *testing.T) {
	dir := stubDir(t)
	cmd := exec.Command("true", "-v")
	if err := Run(context.Background(), cmd, ""); err != nil {
		t.Fatal(err)
	}
	if len(cmd.Args) != 2 {
		t.Fatalf("args = %v, want them untouched", cmd.Args)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a host command left a record: %v", entries)
	}
}

// The watcher kills what finished runs and runs of a lerd killed outright left
// behind, one exec per container, and never touches a run still in progress.
func TestSweep_killsWhatEndedOrOrphanedRunsLeft(t *testing.T) {
	dir := stubDir(t)
	killed := stubKill(t, nil)
	self, start := os.Getpid(), startTimeFn(os.Getpid())
	for _, r := range []record{
		{Container: "lerd-php85-fpm", Marker: "running", PID: self, Start: start},
		{Container: "lerd-php85-fpm", Marker: "ended", PID: self, Start: start, Ended: true},
		{Container: "lerd-php85-fpm", Marker: "orphan", PID: self, Start: "not-its-start-time"},
		{Container: "lerd-php84-fpm", Marker: "other", PID: self, Start: start, Ended: true},
	} {
		if _, err := writeRecord(dir, r); err != nil {
			t.Fatal(err)
		}
	}

	Sweep()

	slices.Sort(*killed)
	want := []string{"lerd-php84-fpm other", "lerd-php85-fpm ended,orphan"}
	if !slices.Equal(*killed, want) && !slices.Equal(*killed, []string{"lerd-php84-fpm other", "lerd-php85-fpm orphan,ended"}) {
		t.Fatalf("killed %v, want %v", *killed, want)
	}
	recs := readRecords(t, dir)
	if len(recs) != 1 || recs[0].Marker != "running" {
		t.Fatalf("records left %+v, want only the running one", recs)
	}
}
