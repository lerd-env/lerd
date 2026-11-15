package watcher

import (
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A service idle-suspend put to sleep is woken by the engine, which also
// restores its sites, before it is ensured; any other is only ensured.
func TestEnsureForCaller_wakesASleepingServiceThroughTheEngine(t *testing.T) {
	var calls []string
	prevAsleep, prevWake, prevEnsure := serviceAsleep, wakeSocketWakeAsleep, wakeSocketEnsureRunning
	t.Cleanup(func() {
		serviceAsleep, wakeSocketWakeAsleep, wakeSocketEnsureRunning = prevAsleep, prevWake, prevEnsure
	})
	serviceAsleep = func(name string) bool { return name == "mysql" }
	wakeSocketWakeAsleep = func(names []string) error { calls = append(calls, "wake "+names[0]); return nil }
	wakeSocketEnsureRunning = func(name string) error { calls = append(calls, "ensure "+name); return nil }

	if err := ensureForCaller("mysql"); err != nil {
		t.Fatal(err)
	}
	if err := ensureForCaller("redis"); err != nil {
		t.Fatal(err)
	}
	want := []string{"wake mysql", "ensure mysql", "ensure redis"}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}

	wakeSocketEnsureRunning = func(string) error { return errors.New("mysql would not start") }
	if err := ensureForCaller("redis"); err == nil || err.Error() != "mysql would not start" {
		t.Errorf("err = %v, want the start's error", err)
	}
}

// A wake that fails is the answer: the request does not start the service
// again, and requests arriving together share that one attempt.
func TestEnsureForCaller_sharesAFailedWake(t *testing.T) {
	prevAsleep, prevWake, prevEnsure := serviceAsleep, wakeSocketWakeAsleep, wakeSocketEnsureRunning
	t.Cleanup(func() {
		serviceAsleep, wakeSocketWakeAsleep, wakeSocketEnsureRunning = prevAsleep, prevWake, prevEnsure
	})
	serviceAsleep = func(string) bool { return true }
	var wakes, ensures atomic.Int32
	wakeSocketWakeAsleep = func([]string) error {
		wakes.Add(1)
		time.Sleep(100 * time.Millisecond)
		return errors.New("image gone")
	}
	wakeSocketEnsureRunning = func(string) error { ensures.Add(1); return nil }

	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = ensureForCaller("mysql") }()
	}
	wg.Wait()
	for i, err := range errs {
		if err == nil || err.Error() != "image gone" {
			t.Errorf("request %d got %v, want the wake's error", i, err)
		}
	}
	if wakes.Load() != 1 || ensures.Load() != 0 {
		t.Errorf("wakes=%d ensures=%d, want one wake and no start after it failed", wakes.Load(), ensures.Load())
	}
}
