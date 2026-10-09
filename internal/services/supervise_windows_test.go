//go:build windows

package services

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The test binary stands in for lerd.exe: run with one of these markers as its
// first argument it plays the supervisor, or a child that fails a set number of
// times, instead of running the tests.
const (
	asSupervisor = "lerd-test-supervise"
	asFlakyChild = "lerd-test-flaky"
)

func TestMain(m *testing.M) {
	// Set before the role switch: a detaching supervisor starts the real one
	// through this too.
	supervisorCommand = func() []string { return []string{os.Args[0], asSupervisor} }
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case asSupervisor:
			os.Exit(runTestSupervisor(os.Args[2:]))
		case asFlakyChild:
			os.Exit(runFlakyChild(os.Args[2:]))
		}
	}
	os.Exit(m.Run())
}

// runTestSupervisor parses the flags supervisedArgs emits, as the cobra
// command does for lerd.exe.
func runTestSupervisor(args []string) int {
	var unit, restart string
	var detach bool
	for len(args) > 0 && args[0] != "--" {
		switch args[0] {
		case "--unit":
			unit, args = args[1], args[2:]
		case "--restart":
			restart, args = args[1], args[2:]
		case "--detach":
			detach, args = true, args[1:]
		default:
			return 2
		}
	}
	if err := Supervise(unit, restart, args[1:], detach); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// runFlakyChild counts its runs in a file and exits 3 until the count reaches
// the limit, then 0.
func runFlakyChild(args []string) int {
	counter := args[0]
	limit, _ := strconv.Atoi(args[1])
	n := 0
	if data, err := os.ReadFile(counter); err == nil {
		n, _ = strconv.Atoi(string(data))
	}
	n++
	_ = os.WriteFile(counter, []byte(strconv.Itoa(n)), 0o644)
	if n < limit {
		return 3
	}
	return 0
}

func flakyArgs(t *testing.T, succeedOn int) (args []string, counter string) {
	t.Helper()
	counter = filepath.Join(t.TempDir(), "runs")
	return []string{os.Args[0], asFlakyChild, counter, strconv.Itoa(succeedOn)}, counter
}

func TestShouldRestart(t *testing.T) {
	cases := []struct {
		policy keepAlivePolicy
		code   int
		want   bool
	}{
		{keepAliveAlways, 0, true},
		{keepAliveAlways, 1, true},
		{keepAliveOnFailure, 0, false},
		{keepAliveOnFailure, 1, true},
		{keepAliveOnFailure, -1, true},
		{keepAliveNever, 1, false},
	}
	for _, c := range cases {
		if got := shouldRestart(c.policy, c.code); got != c.want {
			t.Errorf("shouldRestart(%s, %d) = %v, want %v", c.policy, c.code, got, c.want)
		}
	}
}

func TestNextDelayBacksOffAndResets(t *testing.T) {
	var d time.Duration
	var got []time.Duration
	for i := 0; i < 8; i++ {
		d = nextDelay(d, 0)
		got = append(got, d)
	}
	want := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60}
	for i := range want {
		want[i] *= time.Second
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("delays after quick exits = %v, want %v", got, want)
	}
	if d = nextDelay(d, superviseHealthyRun); d != superviseMinDelay {
		t.Errorf("delay after a healthy run = %v, want %v", d, superviseMinDelay)
	}
}

func TestParseKeepAlivePolicyRoundTrips(t *testing.T) {
	for _, p := range []keepAlivePolicy{keepAliveNever, keepAliveAlways, keepAliveOnFailure} {
		got, err := parseKeepAlivePolicy(p.String())
		if err != nil || got != p {
			t.Errorf("parseKeepAlivePolicy(%q) = %v, %v", p.String(), got, err)
		}
	}
	if _, err := parseKeepAlivePolicy("sometimes"); err == nil {
		t.Error("an unknown policy should be refused")
	}
}

func TestSupervisedArgs(t *testing.T) {
	cmd := []string{`C:\lerd\lerd.exe`, "dns-serve"}
	if got := supervisedArgs("lerd-dns", keepAliveNever, cmd); !reflect.DeepEqual(got, cmd) {
		t.Errorf("a unit with no restart policy should run as is, got %v", got)
	}
	got := supervisedArgs("lerd-dns", keepAliveAlways, cmd)
	want := append(supervisorCommand(), "--unit", "lerd-dns", "--restart", "always", "--detach", "--", `C:\lerd\lerd.exe`, "dns-serve")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("supervisedArgs = %v, want %v", got, want)
	}
}

func TestSuperviseRestartsOnFailureUntilCleanExit(t *testing.T) {
	isolateWinData(t)
	args, counter := flakyArgs(t, 3)
	var log bytes.Buffer
	var slept []time.Duration
	if err := supervise("lerd-test-flaky", "on-failure", args, &log, func(d time.Duration) { slept = append(slept, d) }); err != nil {
		t.Fatal(err)
	}
	if runs, _ := os.ReadFile(counter); string(runs) != "3" {
		t.Errorf("child ran %s times, want 3", runs)
	}
	if want := []time.Duration{time.Second, 2 * time.Second}; !reflect.DeepEqual(slept, want) {
		t.Errorf("waits = %v, want %v", slept, want)
	}
	if n := strings.Count(log.String(), "restarting in"); n != 2 {
		t.Errorf("log notes %d restarts, want 2:\n%s", n, log.String())
	}
	if !strings.Contains(log.String(), "exited with code 0, not restarting") {
		t.Errorf("log does not note the final clean exit:\n%s", log.String())
	}
}

func TestSuperviseAlwaysRestartsAfterCleanExit(t *testing.T) {
	isolateWinData(t)
	args, counter := flakyArgs(t, 0) // exits 0 every run
	var log bytes.Buffer
	sleeps := 0
	stop := "stop supervising"
	defer func() {
		if r := recover(); r != stop {
			t.Fatalf("supervise returned instead of restarting: %v\n%s", r, log.String())
		}
		if runs, _ := os.ReadFile(counter); string(runs) != "2" {
			t.Errorf("child ran %s times, want 2", runs)
		}
	}()
	_ = supervise("lerd-test-always", "always", args, &log, func(time.Duration) {
		if sleeps++; sleeps == 2 {
			panic(stop)
		}
	})
}

func TestSuperviseReleasesOnlyItsOwnPIDFile(t *testing.T) {
	isolateWinData(t)
	if err := os.MkdirAll(unitsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	args, _ := flakyArgs(t, 0)

	_ = os.WriteFile(pidPath("lerd-test-own"), []byte(strconv.Itoa(os.Getpid())), 0o644)
	if err := supervise("lerd-test-own", "on-failure", args, &bytes.Buffer{}, func(time.Duration) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pidPath("lerd-test-own")); !os.IsNotExist(err) {
		t.Error("a supervisor that finished should drop the pid file naming it")
	}

	_ = os.WriteFile(pidPath("lerd-test-other"), []byte("4242"), 0o644)
	if err := supervise("lerd-test-other", "on-failure", args, &bytes.Buffer{}, func(time.Duration) {}); err != nil {
		t.Fatal(err)
	}
	if readPID("lerd-test-other") != 4242 {
		t.Error("a pid file naming another process must be left alone")
	}
}

// childPIDs lists the processes whose parent is pid, leaving out the conhost
// that owns the supervisor's hidden console.
func childPIDs(t *testing.T, pid int) []int {
	t.Helper()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snap) //nolint:errcheck
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []int
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if int(e.ParentProcessID) == pid && !strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "conhost.exe") {
			out = append(out, int(e.ProcessID))
		}
	}
	return out
}

func terminate(t *testing.T, pid int) {
	t.Helper()
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	if err := windows.TerminateProcess(h, 1); err != nil {
		t.Fatal(err)
	}
}

func TestWinSupervisedUnitComesBackAfterACrash(t *testing.T) {
	isolateWinData(t)
	m := &windowsServiceManager{}
	name := "lerd-test-supervised"
	if err := m.WriteServiceUnit(name, sleeperUnit(t)); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(name) })

	supervisor := readPID(name)
	var first int
	waitFor(t, "supervised child", func() bool {
		kids := childPIDs(t, supervisor)
		if len(kids) == 1 {
			first = kids[0]
		}
		return first != 0
	})

	terminate(t, first) // a crash: non-zero exit, not through Stop
	var second int
	for i := 0; i < 50 && second == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		if kids := childPIDs(t, supervisor); len(kids) == 1 && kids[0] != first {
			second = kids[0]
		}
	}
	if second == 0 {
		t.Fatal("the supervisor did not start the unit again after it crashed")
	}
	if got, _ := m.UnitStatus(name); got != "active" {
		t.Errorf("status after the restart = %q, want active", got)
	}

	if err := m.Stop(name); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "supervisor and child gone", func() bool { return !pidAlive(supervisor) && !pidAlive(second) })
	time.Sleep(1500 * time.Millisecond)
	if kids := childPIDs(t, supervisor); len(kids) != 0 {
		t.Errorf("a stopped unit came back: %v", kids)
	}
}

// parentPID returns pid's recorded parent, or 0 when pid is not running.
func parentPID(t *testing.T, pid int) int {
	t.Helper()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snap) //nolint:errcheck
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if int(e.ProcessID) == pid {
			return int(e.ParentProcessID)
		}
	}
	return 0
}

// A unit must not sit in the process tree of whatever started it: the
// dashboard starts workers from serve-ui, and stopping serve-ui is a tree kill.
func TestWinSupervisedUnitHasNoLiveParent(t *testing.T) {
	isolateWinData(t)
	m := &windowsServiceManager{}
	name := "lerd-test-detached"
	if err := m.WriteServiceUnit(name, sleeperUnit(t)); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(name) })

	supervisor := readPID(name)
	if !pidAlive(supervisor) {
		t.Fatal("the recorded pid is not a running supervisor")
	}
	if parent := parentPID(t, supervisor); parent == os.Getpid() || pidAlive(parent) {
		t.Errorf("supervisor %d has live parent %d, so a tree kill of its starter would end it", supervisor, parent)
	}
	if !m.IsActive(name) {
		t.Error("a detached unit should still read as active")
	}
}
