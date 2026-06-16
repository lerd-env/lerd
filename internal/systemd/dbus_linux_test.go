//go:build linux

package systemd

import "testing"

// withServiceSuffix is the gate every DBus unit op passes through. A bug here
// breaks Start/Stop/Restart/Enable/Disable/IsEnabled for every caller, so the
// table covers the inputs each callsite actually emits.
func TestWithServiceSuffix(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"lerd-ui", "lerd-ui.service"},
		{"lerd-watcher", "lerd-watcher.service"},
		{"lerd-queue-myapp", "lerd-queue-myapp.service"},
		{"lerd-ui.service", "lerd-ui.service"},
		{"lerd-test.timer", "lerd-test.timer"},
		{"lerd-stripe-my.app", "lerd-stripe-my.app"},
		{"", ".service"},
	}
	for _, tc := range cases {
		if got := withServiceSuffix(tc.in); got != tc.want {
			t.Errorf("withServiceSuffix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// withDefaultSuffix is used by property lookups that may legitimately target
// a .timer or a .service. Bare names default to .service; explicit suffixes
// pass through verbatim so IsTimerActive's name+".timer" composition works.
func TestWithDefaultSuffix(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"lerd-ui", "lerd-ui.service"},
		{"lerd-test.timer", "lerd-test.timer"},
		{"lerd-test.service", "lerd-test.service"},
		{"lerd-queue-myapp.timer", "lerd-queue-myapp.timer"},
		{"some.weird.name", "some.weird.name"},
	}
	for _, tc := range cases {
		if got := withDefaultSuffix(tc.in); got != tc.want {
			t.Errorf("withDefaultSuffix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// IsTimerActive composes name+".timer" and asks DBus. The suffix rule must
// pass ".timer" through unchanged so IsTimerActive doesn't end up querying
// "lerd-foo.timer.service".
func TestTimerSuffixIsNotDoubleAppended(t *testing.T) {
	if got := withServiceSuffix("lerd-foo.timer"); got != "lerd-foo.timer" {
		t.Errorf("withServiceSuffix(%q) = %q, want passthrough — IsTimerActive would query the wrong unit", "lerd-foo.timer", got)
	}
	if got := withDefaultSuffix("lerd-foo.timer"); got != "lerd-foo.timer" {
		t.Errorf("withDefaultSuffix(%q) = %q, want passthrough", "lerd-foo.timer", got)
	}
}

// NotifyReady and NotifyStopping must be safe to call outside a systemd
// notify-socket context: bare CLI runs (lerd serve-ui from a terminal,
// go test) have no $NOTIFY_SOCKET and the underlying daemon.SdNotify
// returns (false, nil) — no panic, no error surfaced.
func TestNotifyReadyAndStoppingAreSafeWithoutSocket(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "")
	NotifyReady()
	NotifyStopping()
}

// runUnitOpWithRetry is the stop-reliability fix for `lerd stop` leaving a
// container running with "stop … failed: canceled" (a parallel "replace" stop
// of an interdependent unit). It must re-issue only on "canceled", stop as
// soon as the job is "done", give up after maxAttempts, and never swallow a
// transport error. A nil settle (and the production settle being skipped here)
// keeps the test instant.
func TestRunUnitOpWithRetry(t *testing.T) {
	t.Run("canceled then done", func(t *testing.T) {
		results := []string{"canceled", "canceled", "done"}
		var calls int
		got, err := runUnitOpWithRetry(stopRetryAttempts, nil, func() (string, error) {
			r := results[calls]
			calls++
			return r, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "done" {
			t.Fatalf("result = %q, want done", got)
		}
		if calls != 3 {
			t.Fatalf("calls = %d, want 3 (retried until done)", calls)
		}
	})

	t.Run("gives up after maxAttempts and returns last result", func(t *testing.T) {
		var calls int
		got, err := runUnitOpWithRetry(3, nil, func() (string, error) {
			calls++
			return "canceled", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "canceled" {
			t.Fatalf("result = %q, want canceled (final result surfaced)", got)
		}
		if calls != 3 {
			t.Fatalf("calls = %d, want 3 (bounded by maxAttempts)", calls)
		}
	})

	t.Run("non-canceled non-done result is not retried", func(t *testing.T) {
		var calls int
		got, err := runUnitOpWithRetry(stopRetryAttempts, nil, func() (string, error) {
			calls++
			return "failed", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "failed" || calls != 1 {
			t.Fatalf("result=%q calls=%d, want failed/1 (no retry on hard failure)", got, calls)
		}
	})

	t.Run("done on first attempt", func(t *testing.T) {
		var calls int
		got, _ := runUnitOpWithRetry(stopRetryAttempts, nil, func() (string, error) {
			calls++
			return "done", nil
		})
		if got != "done" || calls != 1 {
			t.Fatalf("result=%q calls=%d, want done/1", got, calls)
		}
	})

	t.Run("transport error is surfaced immediately", func(t *testing.T) {
		var calls int
		_, err := runUnitOpWithRetry(stopRetryAttempts, nil, func() (string, error) {
			calls++
			return "", errUnitOpTimedOut
		})
		if err == nil || calls != 1 {
			t.Fatalf("err=%v calls=%d, want non-nil/1 (no retry on transport error)", err, calls)
		}
	})

	t.Run("maxAttempts below one is clamped to a single attempt", func(t *testing.T) {
		var calls int
		runUnitOpWithRetry(0, nil, func() (string, error) { //nolint:errcheck
			calls++
			return "canceled", nil
		})
		if calls != 1 {
			t.Fatalf("calls = %d, want 1 (clamped)", calls)
		}
	})
}
