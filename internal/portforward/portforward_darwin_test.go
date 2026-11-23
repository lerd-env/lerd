//go:build darwin

package portforward

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// fakeRoot stands in for sudo and the reachability probe.
func fakeRoot(t *testing.T) *[]string {
	t.Helper()
	isolate(t)
	prevSudo, prevLegacy, prevReaches, prevLoaded := runSudo, removeLegacy, reaches, loaded
	var calls []string
	runSudo = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		if args[0] == "rm" {
			for _, p := range args[2:] {
				os.Remove(p)
			}
		}
		return nil
	}
	removeLegacy = func() error { calls = append(calls, "remove pf"); return nil }
	reaches = func(int) bool { return true }
	loaded = func(int) bool { return true }
	t.Cleanup(func() { runSudo, removeLegacy, reaches, loaded = prevSudo, prevLegacy, prevReaches, prevLoaded })
	return &calls
}

func fakeWrite(path string, content []byte, mode os.FileMode) error {
	return os.WriteFile(path, content, mode)
}

func TestInstallReplacesPfWithALaunchdJobPerPort(t *testing.T) {
	calls := fakeRoot(t)
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0] != "remove pf" {
		t.Errorf("calls = %q, want the legacy pf rule removed before any job loads", *calls)
	}
	for _, p := range []int{80, 443} {
		if !slices.Contains(*calls, "launchctl bootstrap system "+PlistPath(p)) {
			t.Errorf("calls = %q, want %d bootstrapped", *calls, p)
		}
	}
	if got := Redirected(); !got[80] || !got[443] {
		t.Errorf("Redirected() = %v, want 80 and 443", got)
	}
}

func TestInstallLeavesACurrentJobAlone(t *testing.T) {
	calls := fakeRoot(t)
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	*calls = nil
	if Pending("alice") {
		t.Error("nothing should be pending after an install")
	}
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	for _, c := range *calls {
		if strings.HasPrefix(c, "launchctl bootstrap") {
			t.Errorf("calls = %q, want no reload of a current job", *calls)
		}
	}
}

// Moving nginx above 1024 must take the jobs for the old ports down, or they
// keep holding 80 and 443 for nothing.
func TestInstallRemovesJobsForPortsNginxLeft(t *testing.T) {
	fakeRoot(t)
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Nginx.HTTPPort, cfg.Nginx.HTTPSPort = 8080, 8443
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	for _, p := range []int{80, 443} {
		if _, err := os.Stat(PlistPath(p)); err == nil {
			t.Errorf("job for %d stayed behind", p)
		}
	}
}

// A forward that never reaches nginx must not stay, or nginx moves to a port
// nothing serves and every site goes down.
func TestInstallTakesTheJobsBackWhenLoopbackDoesNotReachThem(t *testing.T) {
	fakeRoot(t)
	reaches = func(int) bool { return false }
	moved, err := Install("alice", fakeWrite)
	if err == nil {
		t.Fatal("an unreachable forward must be reported")
	}
	if moved || len(Redirected()) != 0 {
		t.Errorf("moved = %v with %v redirected, want nothing left", moved, Redirected())
	}
}

func TestRemoveTakesDownTheJobsAndTheLegacyRule(t *testing.T) {
	calls := fakeRoot(t)
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	*calls = nil
	if err := Remove(); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(*calls, "launchctl bootout system/"+Label(80)) || !slices.Contains(*calls, "remove pf") {
		t.Errorf("calls = %q, want the jobs booted out and pf removed", *calls)
	}
	if len(Redirected()) != 0 {
		t.Error("nothing should be redirected after Remove")
	}
}

// Without a terminal the password prompt fails; the old pf rule must still be
// there afterwards, or nothing forwards 80 and 443 and every site goes down.
func TestInstallKeepsTheLegacyRuleWhenThePasswordPromptFails(t *testing.T) {
	calls := fakeRoot(t)
	failWrite := func(string, []byte, os.FileMode) error { return errors.New("a terminal is required") }
	if _, err := Install("alice", failWrite); err == nil {
		t.Fatal("a failed write must be reported")
	}
	if slices.Contains(*calls, "remove pf") {
		t.Errorf("calls = %q, want the pf rule left alone", *calls)
	}
}

// An install cut off between writing a job and loading it leaves a current
// file behind; the next install has to load it, not skip it.
func TestInstallLoadsAJobWhoseFileIsCurrentButNotLoaded(t *testing.T) {
	calls := fakeRoot(t)
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	*calls = nil
	loaded = func(p int) bool { return p != 443 }
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(*calls, "launchctl bootstrap system "+PlistPath(443)) || slices.Contains(*calls, "launchctl bootstrap system "+PlistPath(80)) {
		t.Errorf("calls = %q, want only 443 loaded", *calls)
	}
}

// A job file counts as a forward, so one left behind by a failed later write
// would move nginx to a port nothing serves.
func TestInstallRollsBackJobsItWroteWhenALaterWriteFails(t *testing.T) {
	calls := fakeRoot(t)
	failOn443 := func(path string, content []byte, mode os.FileMode) error {
		if path == PlistPath(443) {
			return errors.New("disk full")
		}
		return fakeWrite(path, content, mode)
	}
	if _, err := Install("alice", failOn443); err == nil {
		t.Fatal("a failed write must be reported")
	}
	if len(Redirected()) != 0 || slices.Contains(*calls, "remove pf") {
		t.Errorf("Redirected() = %v with calls %q, want nothing left and pf untouched", Redirected(), *calls)
	}
}

func TestPendingWhileAJobIsNotLoaded(t *testing.T) {
	fakeRoot(t)
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	loaded = func(p int) bool { return p != 80 }
	if !Pending("alice") {
		t.Error("an unloaded job leaves root work to do")
	}
}
