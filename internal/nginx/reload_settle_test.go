package nginx

import (
	"errors"
	"testing"
	"time"
)

// `nginx -s reload` signals the master and returns. The master forks workers on
// the new configuration but leaves the old ones serving until they drain, so for
// a moment both generations are alive and a request can land on either. That is
// what made `lerd domain add` report a domain it could not yet serve, and
// `lerd domain remove` go on serving one it had just dropped: measured on the
// guest, the old workers were still listed alongside the new ones and the very
// next request answered 404 on http and failed TLS outright.
//
// Waiting for the previous generation to retire is the only honest signal that
// the configuration a caller just wrote is the one every request will now meet.
func TestReloadAndSettleWaitsForTheOldWorkersToRetire(t *testing.T) {
	reloads := 0
	generations := [][]string{
		{"82", "83", "87", "88"},   // still only the old ones
		{"87", "88", "100", "101"}, // new ones up, two old ones draining
		{"100", "101", "102"},      // the old generation is gone
	}
	call := 0

	err := reloadAndSettle(
		func() error { reloads++; return nil },
		func() ([]string, error) {
			if call == 0 {
				call++
				return []string{"82", "83", "87", "88"}, nil
			}
			g := generations[min(call, len(generations)-1)]
			call++
			return g, nil
		},
		time.Second, func(time.Duration) {},
	)
	if err != nil {
		t.Fatalf("reloadAndSettle: %v", err)
	}
	if reloads != 1 {
		t.Errorf("reload ran %d times, want exactly 1", reloads)
	}
	if call < 3 {
		t.Errorf("stopped polling after %d looks, so it returned while an old worker was still serving", call)
	}
}

// A worker holding a long connection can outlast any budget. Running out is not
// an error: the configuration on disk is already correct and every later request
// meets it, so failing the command the user typed would be worse than the short
// window it is trying to close.
func TestReloadAndSettleGivesUpWithoutFailing(t *testing.T) {
	stuck := []string{"82", "83"}
	err := reloadAndSettle(
		func() error { return nil },
		func() ([]string, error) { return stuck, nil },
		30*time.Millisecond, func(time.Duration) {},
	)
	if err != nil {
		t.Errorf("a worker that never retires made the command fail: %v", err)
	}
}

// A reload that could not be signalled at all is a real failure and is returned,
// without spending the settle budget on a reload that never happened.
func TestReloadAndSettleReturnsTheReloadFailure(t *testing.T) {
	boom := errors.New("nginx is not running")
	looks := 0
	err := reloadAndSettle(
		func() error { return boom },
		func() ([]string, error) { looks++; return []string{"82"}, nil },
		time.Second, func(time.Duration) {},
	)
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want the reload failure", err)
	}
	// One look is the snapshot of the generation about to be replaced, taken
	// before the reload is attempted. Any more means it settled over a reload
	// that never happened.
	if looks != 1 {
		t.Errorf("read the worker list %d times, want only the pre-reload snapshot", looks)
	}
}

// Not being able to read the worker list is not worth failing over either: the
// reload was signalled, which is the part that matters.
func TestReloadAndSettleToleratesAnUnreadableWorkerList(t *testing.T) {
	err := reloadAndSettle(
		func() error { return nil },
		func() ([]string, error) { return nil, errors.New("no such container") },
		30*time.Millisecond, func(time.Duration) {},
	)
	if err != nil {
		t.Errorf("an unreadable worker list made the command fail: %v", err)
	}
}
