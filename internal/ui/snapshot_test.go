package ui

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// A browser opening the dashboard fires several requests at once. On a cold
// cache the losers used to be handed the nil they found there, which the
// handler wrote out as a zero-byte body and the dashboard read as "no sites".
func TestSnapshotColdCacheServesEveryConcurrentCaller(t *testing.T) {
	var builds int
	slot := &snapshotSlot{fn: func() ([]byte, error) {
		builds++
		time.Sleep(20 * time.Millisecond)
		return []byte(`[{"name":"acme"}]`), nil
	}}

	const callers = 25
	var wg sync.WaitGroup
	results := make([][]byte, callers)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = slot.get()
		}()
	}
	wg.Wait()

	for i, got := range results {
		if len(got) == 0 {
			t.Fatalf("caller %d got an empty snapshot on a cold cache", i)
		}
	}
	if builds != 1 {
		t.Errorf("builds = %d, want 1: callers must queue behind one rebuild", builds)
	}
}

// The reason the losers returned early in the first place: a caller holding a
// usable value should not wait on a slow podman inspect.
func TestSnapshotWarmCacheServesStaleWithoutWaiting(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	slot := &snapshotSlot{data: []byte(`["stale"]`), at: time.Now().Add(-time.Hour)}
	slot.fn = func() ([]byte, error) {
		entered <- struct{}{}
		<-release
		return []byte(`["fresh"]`), nil
	}

	go slot.get()
	<-entered // the rebuild is in flight and holding the build lock

	done := make(chan []byte, 1)
	go func() { done <- slot.get() }()
	select {
	case got := <-done:
		if string(got) != `["stale"]` {
			t.Errorf("got %s, want the stale value", got)
		}
	case <-time.After(time.Second):
		t.Fatal("a caller holding a stale value queued behind the in-flight rebuild")
	}
	close(release)
}

// A transient failure to read the registry must not be stored with a fresh
// timestamp, or it is served as a legitimate empty list for a whole TTL.
func TestSnapshotDoesNotCacheAFailedBuild(t *testing.T) {
	fail := true
	slot := &snapshotSlot{fn: func() ([]byte, error) {
		if fail {
			return nil, errors.New("reading sites.yaml: permission denied")
		}
		return []byte(`[{"name":"acme"}]`), nil
	}}

	if got := slot.get(); got != nil {
		t.Fatalf("failed build returned %s, want nil so the handler can say so", got)
	}

	fail = false
	if got := slot.get(); string(got) != `[{"name":"acme"}]` {
		t.Fatalf("got %s, want the rebuilt value: the failure was cached", got)
	}
}

// With something cached, a failed rebuild is better answered with the stale
// value than with nothing.
func TestSnapshotKeepsStaleWhenRebuildFails(t *testing.T) {
	slot := &snapshotSlot{data: []byte(`["stale"]`), at: time.Now().Add(-time.Hour)}
	slot.fn = func() ([]byte, error) { return nil, errors.New("registry unreadable") }

	if got := slot.get(); string(got) != `["stale"]` {
		t.Fatalf("got %s, want the stale value kept through a failed rebuild", got)
	}
	// The stale timestamp must not be refreshed, so the next call retries.
	var attempts int
	slot.fn = func() ([]byte, error) {
		attempts++
		return []byte(`["fresh"]`), nil
	}
	if got := slot.get(); string(got) != `["fresh"]` {
		t.Fatalf("got %s, want a retry after the failure", got)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestSnapshotServesCachedValueWithinTTL(t *testing.T) {
	var builds int
	slot := &snapshotSlot{fn: func() ([]byte, error) {
		builds++
		return []byte(`["v"]`), nil
	}}

	slot.get()
	slot.get()
	if builds != 1 {
		t.Errorf("builds = %d, want 1 within the TTL", builds)
	}

	slot.invalidate()
	slot.get()
	if builds != 2 {
		t.Errorf("builds = %d after invalidate, want 2", builds)
	}
}
