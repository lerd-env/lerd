package podman

import (
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestFPMUnitName(t *testing.T) {
	for version, want := range map[string]string{
		"8.5": "lerd-php85-fpm",
		"8.4": "lerd-php84-fpm",
		"7.4": "lerd-php74-fpm",
	} {
		if got := FPMUnitName(version); got != want {
			t.Errorf("FPMUnitName(%q) = %q, want %q", version, got, want)
		}
	}
}

// The probe has to run inside the container and must not depend on tooling an
// FPM image may not carry, so it goes through PHP itself.
func TestFPMReadyProbeUsesPHP(t *testing.T) {
	if len(fpmReadyProbe) == 0 || fpmReadyProbe[0] != "php" {
		t.Fatalf("probe should run through php, got %v", fpmReadyProbe)
	}
	joined := ""
	for _, a := range fpmReadyProbe {
		joined += a + " "
	}
	if !contains(joined, "9000") {
		t.Errorf("probe does not test the fastcgi port: %v", fpmReadyProbe)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Restarting a PHP pool gives its container a new address on the lerd network,
// and nginx goes on connecting to the previous one until its own timeout. The
// restart used to return as soon as systemd reported its job done, roughly a
// third of a second, over a pool that was not yet accepting:
//
//	xdebug off: command returned after 0.35s, first request 504 after 60.2s
//
// so the command printed a tick over a site that was down. Only the per-version
// FPM containers wait; nothing else sits behind nginx the same way.
func TestIsFPMUnit(t *testing.T) {
	for _, u := range []string{"lerd-php85-fpm", "lerd-php74-fpm"} {
		if !isFPMUnit(u) {
			t.Errorf("isFPMUnit(%q) = false, want true", u)
		}
	}
	for _, u := range []string{"lerd-nginx", "lerd-mysql", "lerd-dns", "lerd-queue-demo"} {
		if isFPMUnit(u) {
			t.Errorf("isFPMUnit(%q) = true, want false", u)
		}
	}
}

// A pool that never comes back must not hang the command forever, and unlike
// EnsureFPMReady the restart path has to report the timeout rather than swallow
// it: the caller is about to tell the user the restart succeeded.
func TestWaitFPMAcceptingReportsATimeout(t *testing.T) {
	prev := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}
	defer func() { execCommand = prev }()

	err := waitFPMAccepting("lerd-php85-fpm", 100*time.Millisecond)
	if err == nil {
		t.Fatal("waitFPMAccepting returned nil for a pool that never accepted")
	}
	if !errors.Is(err, errFPMNotAccepting) {
		t.Errorf("error %v does not wrap errFPMNotAccepting", err)
	}
	if !contains(err.Error(), "lerd-php85-fpm") {
		t.Errorf("error does not name the unit: %q", err.Error())
	}
}

// The happy path returns as soon as the probe passes.
func TestWaitFPMAcceptingReturnsWhenThePoolAnswers(t *testing.T) {
	prev := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("true")
	}
	defer func() { execCommand = prev }()

	if err := waitFPMAccepting("lerd-php85-fpm", 2*time.Second); err != nil {
		t.Fatalf("waitFPMAccepting returned %v, want nil", err)
	}
}

// The pool being back is only half of it: nginx still holds the address the
// container had before the restart, and a request landing in that window waits
// out fastcgi_connect_timeout and answers 504. Measured on the guest across ten
// consecutive xdebug toggles: five failures without this reload, none with it.
// Only FPM units trigger it, so restarting a service or a worker does not
// bounce nginx's config for no reason.
func TestFPMRestartDropsTheNginxUpstreamCache(t *testing.T) {
	prevProbe := execCommand
	execCommand = func(string, ...string) *exec.Cmd { return exec.Command("true") }
	defer func() { execCommand = prevProbe }()

	called := 0
	prevDrop := dropNginxUpstreamCache
	dropNginxUpstreamCache = func() { called++ }
	defer func() { dropNginxUpstreamCache = prevDrop }()

	if err := awaitFPM("lerd-php85-fpm"); err != nil {
		t.Fatalf("awaitFPM: %v", err)
	}
	if called != 1 {
		t.Errorf("nginx upstream cache dropped %d times after an FPM restart, want 1", called)
	}

	called = 0
	if err := awaitFPM("lerd-mysql"); err != nil {
		t.Fatalf("awaitFPM on a non-FPM unit: %v", err)
	}
	if called != 0 {
		t.Errorf("restarting %s bounced nginx %d times, want 0", "lerd-mysql", called)
	}
}
