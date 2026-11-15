package watcher

import (
	"errors"
	"slices"
	"testing"
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
	wakeSocketWakeAsleep = func(names []string) { calls = append(calls, "wake "+names[0]) }
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
