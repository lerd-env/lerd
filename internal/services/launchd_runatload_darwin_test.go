package services

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

// bootstrap with RunAtLoad=true is supposed to start the job, but launchd does
// not always oblige: after an abrupt teardown the job loads and sits at "not
// running" while lerd reports success, which is how every worker ended up
// failed with nothing wrong in its own log.
func TestRunAtLoadIsVerifiedAndKicked(t *testing.T) {
	var kicked []string
	running := false

	err := ensureStarted("lerd-horizon-app",
		func(string) bool { return running },
		func(label string) error { kicked = append(kicked, label); running = true; return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kicked) != 1 {
		t.Errorf("a loaded-but-idle job must be kicked, got %v", kicked)
	}
}

// A job bootstrap really did start must not be kicked: that would restart a
// healthy worker for nothing.
func TestRunningJobIsLeftAlone(t *testing.T) {
	var kicked []string
	err := ensureStarted("lerd-horizon-app",
		func(string) bool { return true },
		func(label string) error { kicked = append(kicked, label); return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kicked) != 0 {
		t.Errorf("a running job must be left alone, got %v", kicked)
	}
}

// A kick that fails surfaces, rather than reporting a start that never happened.
func TestKickFailureSurfaces(t *testing.T) {
	err := ensureStarted("lerd-horizon-app",
		func(string) bool { return false },
		func(string) error { return errors.New("boom") })
	if err == nil {
		t.Error("a failed kick must be reported, not swallowed")
	}
}

func TestWaitForPollsUntilConditionHolds(t *testing.T) {
	polls := 0
	var slept time.Duration
	ok := waitFor(func() bool { polls++; return polls == 3 }, time.Second, func(d time.Duration) { slept += d })
	if !ok || polls != 3 || slept == 0 {
		t.Errorf("expected to poll until the condition held, ok=%v polls=%d slept=%v", ok, polls, slept)
	}
}

// Each check runs launchctl, which can be slow, so the wait ends at its timeout
// in real time rather than only after enough sleeps.
func TestWaitForStopsAtWallClockTimeout(t *testing.T) {
	checks := 0
	ok := waitFor(func() bool { checks++; time.Sleep(200 * time.Millisecond); return false },
		400*time.Millisecond, func(time.Duration) {})
	if ok || checks > 3 {
		t.Errorf("slow checks must not stretch the wait past its timeout, ran %d checks", checks)
	}
}

func TestWaitForGivesUp(t *testing.T) {
	var slept time.Duration
	if waitFor(func() bool { return false }, time.Second, func(d time.Duration) { slept += d }) {
		t.Error("a condition that never holds must report false")
	}
	if slept < time.Second || slept > time.Second+pollInterval {
		t.Errorf("the wait must stop at its timeout, waited %v", slept)
	}
}

// bootout returns while the job is still exiting. Bootstrapping then fails and
// the kickstart fallback lands on the dying job, so Start reported success and
// launchd dropped the job moments later, leaving no watcher at all.
func TestStartRefusesWhileBootedOutJobStillExits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(launchAgentsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plistPath("lerd-watcher"), []byte("<key>RunAtLoad</key>"), 0o644); err != nil {
		t.Fatal(err)
	}
	prevLaunchctl, prevSleep := launchctl, sleep
	t.Cleanup(func() { launchctl, sleep = prevLaunchctl, prevSleep })
	sleep = func(time.Duration) {}
	var verbs []string
	launchctl = func(args ...string) ([]byte, error) {
		verbs = append(verbs, args[0])
		return nil, nil
	}

	if err := (&darwinServiceManager{}).Start("lerd-watcher"); err == nil {
		t.Fatal("a job that never finished exiting must fail the start, not report success")
	}
	for _, v := range verbs {
		if v == "bootstrap" || v == "kickstart" {
			t.Errorf("must not %s a job that is still exiting, ran %v", v, verbs)
		}
	}
}

// A kick launchd answers with 37 counts only once the job really runs.
func TestKickInProgressMustReachRunning(t *testing.T) {
	prevSleep := sleep
	t.Cleanup(func() { sleep = prevSleep })
	sleep = func(time.Duration) {}
	inProgress := exec.Command("sh", "-c", "exit 37").Run()

	polls := 0
	err := ensureStarted("lerd-watcher",
		func(string) bool { polls++; return polls > 2 },
		func(string) error { return inProgress })
	if err != nil {
		t.Errorf("a job that comes up after 37 is started: %v", err)
	}

	err = ensureStarted("lerd-watcher",
		func(string) bool { return false },
		func(string) error { return inProgress })
	if err == nil {
		t.Error("a job that never comes up after 37 must fail the start")
	}
}

// launchctl prints nothing for 37 (EALREADY), so only the exit code tells it
// apart from a real kickstart failure.
func TestKickstartInProgressReadsExitCode(t *testing.T) {
	exit := func(code string) error { return exec.Command("sh", "-c", "exit "+code).Run() }
	if !kickstartInProgress(exit("37")) {
		t.Error("exit 37 means launchd is already starting the job")
	}
	if kickstartInProgress(exit("1")) || kickstartInProgress(nil) {
		t.Error("only exit 37 counts as already in progress")
	}
}
