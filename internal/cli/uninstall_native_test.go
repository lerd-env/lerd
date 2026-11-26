package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// The launchd sweep addresses jobs as com.lerd.<file>, which the native FPM
// pools are not, so uninstall deleted their plists and left them serving. A
// reinstall then raced the still-loaded job and came up with no FPM at all.
func TestUninstallStopsEveryNativePool(t *testing.T) {
	var stopped []string
	stopNativePools([]string{"8.3", "8.4"}, func(v string) error { stopped = append(stopped, v); return nil })
	if want := []string{"8.3", "8.4"}; !reflect.DeepEqual(stopped, want) {
		t.Errorf("stopped %v, want %v", stopped, want)
	}
}

// The watcher repairs DNS it finds broken, so a teardown that ran while it was
// up got undone a second later and left .test routed to a dead lerd-dns.
func TestUninstallStopsTheWatcherBeforeDNSTeardown(t *testing.T) {
	var calls []string
	err := stopWatcherThenTeardownDNS(
		func(unit string) error { calls = append(calls, "stop "+unit); return nil },
		func(string) bool { return true },
		func() error { calls = append(calls, "teardown"); return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"stop lerd-watcher", "teardown"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("calls %v, want %v", calls, want)
	}
}

func TestUninstallRefusesDNSTeardownWhileTheWatcherStillRuns(t *testing.T) {
	tornDown := false
	err := stopWatcherThenTeardownDNS(
		func(string) error { return errors.New("no user bus") },
		func(string) bool { return false },
		func() error { tornDown = true; return nil },
	)
	if err == nil || tornDown {
		t.Errorf("err %v, tornDown %v: want an error and the DNS setup left alone", err, tornDown)
	}
}

// A watcher that was never running has nothing to heal with, so a failed stop
// must not block the uninstall.
func TestUninstallTearsDNSDownWhenTheWatcherIsAlreadyGone(t *testing.T) {
	tornDown := false
	err := stopWatcherThenTeardownDNS(
		func(string) error { return errors.New("unit not loaded") },
		func(string) bool { return true },
		func() error { tornDown = true; return nil },
	)
	if err != nil || !tornDown {
		t.Errorf("err %v, tornDown %v: want the teardown to run", err, tornDown)
	}
}

// Going on after a failed restore would delete the copy with the data dir.
func TestUninstallStopsWhenDNSTeardownFails(t *testing.T) {
	err := stopWatcherThenTeardownDNS(
		func(string) error { return nil },
		func(string) bool { return true },
		func() error { return errors.New("could not put it back") },
	)
	if err == nil || !strings.Contains(err.Error(), "could not put it back") {
		t.Errorf("err = %v", err)
	}
}

// An unreachable bus answers "unknown", which must not pass for a stopped
// watcher, or the teardown runs while it can still undo it.
func TestConfirmedStoppedOnlyTrustsAStoppedAnswer(t *testing.T) {
	cases := []struct {
		state string
		err   error
		want  bool
	}{
		{"inactive", nil, true},
		{"failed", nil, true},
		{"active", nil, false},
		{"deactivating", nil, false},
		{"unknown", nil, false},
		{"inactive", errors.New("no user bus"), false},
	}
	for _, c := range cases {
		if got := confirmedStopped(c.state, c.err); got != c.want {
			t.Errorf("confirmedStopped(%q, %v) = %v, want %v", c.state, c.err, got, c.want)
		}
	}
}
