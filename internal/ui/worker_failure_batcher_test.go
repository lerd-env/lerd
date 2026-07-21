package ui

import (
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/workerheal"
)

// installBatchTestSink swaps the dispatcher and shrinks the delay so a real
// timer flush completes inside the test, then restores both on cleanup.
func installBatchTestSink(t *testing.T, delay time.Duration) (wait func() [][]workerheal.UnhealthyWorker) {
	t.Helper()
	var mu sync.Mutex
	var calls [][]workerheal.UnhealthyWorker
	done := make(chan struct{}, 8)

	origDispatch := workerFailureDispatch
	origDelay := workerFailureBatchDelay
	workerFailureDispatch = func(ws []workerheal.UnhealthyWorker) {
		mu.Lock()
		copyWs := append([]workerheal.UnhealthyWorker(nil), ws...)
		calls = append(calls, copyWs)
		mu.Unlock()
		done <- struct{}{}
	}
	workerFailureBatchDelay = delay
	t.Cleanup(func() {
		workerFailureDispatch = origDispatch
		workerFailureBatchDelay = origDelay
		workerFailureBatchMu.Lock()
		pendingWorkerFailures = map[string]workerheal.UnhealthyWorker{}
		if workerFailureFlushTimer != nil {
			workerFailureFlushTimer.Stop()
			workerFailureFlushTimer = nil
		}
		workerFailureBatchMu.Unlock()
	})
	return func() [][]workerheal.UnhealthyWorker {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for batched dispatch")
		}
		mu.Lock()
		defer mu.Unlock()
		return calls
	}
}

// confirmUnits stubs the flush-time re-check so the listed units still read as
// unhealthy and everything else reads as recovered.
func confirmUnits(t *testing.T, units ...workerheal.UnhealthyWorker) {
	t.Helper()
	orig := workerFailureDetect
	workerFailureDetect = func() ([]workerheal.UnhealthyWorker, error) { return units, nil }
	t.Cleanup(func() { workerFailureDetect = orig })
}

func TestQueueWorkerFailureNotifications_BatchesWithinWindow(t *testing.T) {
	wait := installBatchTestSink(t, 200*time.Millisecond)
	confirmUnits(t,
		uw("lerd-queue-a.service", "a.test", "queue", "failed"),
		uw("lerd-horizon-a.service", "a.test", "horizon", "failed"),
		uw("lerd-scheduler-b.service", "b.test", "scheduler", "failed"),
	)
	queueWorkerFailureNotifications([]workerheal.UnhealthyWorker{
		uw("lerd-queue-a.service", "a.test", "queue", "failed"),
	})
	queueWorkerFailureNotifications([]workerheal.UnhealthyWorker{
		uw("lerd-horizon-a.service", "a.test", "horizon", "failed"),
		uw("lerd-scheduler-b.service", "b.test", "scheduler", "failed"),
	})
	calls := wait()
	if len(calls) != 1 {
		t.Fatalf("expected one grouped dispatch, got %d", len(calls))
	}
	got := calls[0]
	if len(got) != 3 {
		t.Fatalf("expected 3 batched workers, got %d", len(got))
	}
	units := make([]string, len(got))
	for i, w := range got {
		units[i] = w.Unit
	}
	sort.Strings(units)
	want := []string{
		"lerd-horizon-a.service",
		"lerd-queue-a.service",
		"lerd-scheduler-b.service",
	}
	for i := range want {
		if units[i] != want[i] {
			t.Errorf("units[%d] = %q, want %q", i, units[i], want[i])
		}
	}
}

func TestQueueWorkerFailureNotifications_DedupesByUnit(t *testing.T) {
	wait := installBatchTestSink(t, 200*time.Millisecond)
	confirmUnits(t, uw("lerd-queue-a.service", "a.test", "queue", "start-limit-hit"))
	queueWorkerFailureNotifications([]workerheal.UnhealthyWorker{
		uw("lerd-queue-a.service", "a.test", "queue", "failed"),
	})
	queueWorkerFailureNotifications([]workerheal.UnhealthyWorker{
		uw("lerd-queue-a.service", "a.test", "queue", "start-limit-hit"),
	})
	calls := wait()
	if len(calls) != 1 || len(calls[0]) != 1 {
		t.Fatalf("expected single dedup'd dispatch, got %+v", calls)
	}
	if calls[0][0].State != "start-limit-hit" {
		t.Errorf("expected last-write-wins state, got %q", calls[0][0].State)
	}
}

func TestQueueWorkerFailureNotifications_SecondBurstArmsFreshWindow(t *testing.T) {
	wait := installBatchTestSink(t, 100*time.Millisecond)
	confirmUnits(t,
		uw("lerd-queue-a.service", "a.test", "queue", "failed"),
		uw("lerd-horizon-b.service", "b.test", "horizon", "failed"),
	)
	queueWorkerFailureNotifications([]workerheal.UnhealthyWorker{
		uw("lerd-queue-a.service", "a.test", "queue", "failed"),
	})
	first := wait()
	if len(first) != 1 || len(first[0]) != 1 || first[0][0].Unit != "lerd-queue-a.service" {
		t.Fatalf("first burst dispatch malformed: %+v", first)
	}
	// After the first batch has fired and cleared state, a fresh failure
	// must arm a brand-new window and dispatch independently. Without this
	// the batcher would silently swallow all post-first-burst notifications.
	queueWorkerFailureNotifications([]workerheal.UnhealthyWorker{
		uw("lerd-horizon-b.service", "b.test", "horizon", "failed"),
	})
	second := wait()
	if len(second) != 2 {
		t.Fatalf("expected two dispatches across two bursts, got %d", len(second))
	}
	if len(second[1]) != 1 || second[1][0].Unit != "lerd-horizon-b.service" {
		t.Errorf("second burst payload wrong: %+v", second[1])
	}
}

// systemd restarts worker units on its own (RestartSec=5), so a worker that
// tripped once is often active again before the window closes. Announcing it
// then sends the user to a dashboard showing nothing wrong.
func TestFlush_SkipsWorkersThatRecoveredDuringWindow(t *testing.T) {
	origDispatch := workerFailureDispatch
	origDelay := workerFailureBatchDelay
	t.Cleanup(func() {
		workerFailureDispatch = origDispatch
		workerFailureBatchDelay = origDelay
	})
	var fired bool
	workerFailureDispatch = func([]workerheal.UnhealthyWorker) { fired = true }
	workerFailureBatchDelay = 50 * time.Millisecond
	confirmUnits(t) // everything came back on its own

	queueWorkerFailureNotifications([]workerheal.UnhealthyWorker{
		uw("lerd-queue-a.service", "a.test", "queue", "failed"),
	})
	time.Sleep(200 * time.Millisecond)
	if fired {
		t.Error("dispatched a notification for a worker that had already recovered")
	}
}

func TestFlush_KeepsWorkersStillDownAndRefreshesState(t *testing.T) {
	wait := installBatchTestSink(t, 50*time.Millisecond)
	// One recovered, the other is still down and has since gone from a crash
	// loop to plain stopped.
	confirmUnits(t, uw("lerd-horizon-b.service", "b.test", "horizon", "expected-but-stopped"))

	queueWorkerFailureNotifications([]workerheal.UnhealthyWorker{
		uw("lerd-queue-a.service", "a.test", "queue", "failed"),
		uw("lerd-horizon-b.service", "b.test", "horizon", "failed"),
	})
	calls := wait()
	if len(calls) != 1 || len(calls[0]) != 1 {
		t.Fatalf("expected one dispatch carrying one worker, got %+v", calls)
	}
	if calls[0][0].Unit != "lerd-horizon-b.service" {
		t.Errorf("dispatched %q, want the worker that stayed down", calls[0][0].Unit)
	}
	if calls[0][0].State != "expected-but-stopped" {
		t.Errorf("State=%q, want the state re-read at flush time", calls[0][0].State)
	}
}

// Failing to confirm is not evidence of recovery, so a broken detector must
// not turn into silence.
func TestFlush_KeepsBatchWhenRecheckFails(t *testing.T) {
	wait := installBatchTestSink(t, 50*time.Millisecond)
	orig := workerFailureDetect
	workerFailureDetect = func() ([]workerheal.UnhealthyWorker, error) {
		return nil, errors.New("systemctl unavailable")
	}
	t.Cleanup(func() { workerFailureDetect = orig })

	queueWorkerFailureNotifications([]workerheal.UnhealthyWorker{
		uw("lerd-queue-a.service", "a.test", "queue", "failed"),
	})
	calls := wait()
	if len(calls) != 1 || len(calls[0]) != 1 {
		t.Fatalf("expected the unconfirmed batch to still dispatch, got %+v", calls)
	}
}

func TestQueueWorkerFailureNotifications_EmptyNoDispatch(t *testing.T) {
	origDispatch := workerFailureDispatch
	defer func() { workerFailureDispatch = origDispatch }()
	var fired bool
	workerFailureDispatch = func([]workerheal.UnhealthyWorker) { fired = true }
	queueWorkerFailureNotifications(nil)
	queueWorkerFailureNotifications([]workerheal.UnhealthyWorker{})
	time.Sleep(20 * time.Millisecond)
	if fired {
		t.Errorf("empty batch should not arm timer or dispatch")
	}
}
