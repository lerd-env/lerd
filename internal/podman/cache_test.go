package podman

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// startLoop runs the cache loop and returns a stop func that cancels it and
// waits for it to exit, also running at test end if the test never calls it.
// The waiting is the point: a bare cancel returns while the goroutine may still
// be inside pollFn, live under whatever the test touches next.
func startLoop(t *testing.T, c *ContainerCache) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.loop(ctx)
		close(done)
	}()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
	t.Cleanup(stop)
	return stop
}

// newTestCache returns a fresh ContainerCache with a custom poll function.
func newTestCache(pollFn func() (string, error)) *ContainerCache {
	return &ContainerCache{
		running:  make(map[string]bool),
		interval: 10 * time.Second,
		refresh:  make(chan struct{}, 1),
		pollFn:   pollFn,
	}
}

func TestPollParsesRunningContainers(t *testing.T) {
	c := newTestCache(func() (string, error) {
		return "lerd-nginx\trunning\nlerd-mysql\texited\nlerd-redis\tRunning Up 2h", nil
	})
	c.started = true
	c.poll()

	if !c.Running("lerd-nginx") {
		t.Error("lerd-nginx should be running")
	}
	if c.Running("lerd-mysql") {
		t.Error("lerd-mysql should not be running (exited)")
	}
	if !c.Running("lerd-redis") {
		t.Error("lerd-redis should be running (Running prefix, mixed case)")
	}
	if c.Running("lerd-postgres") {
		t.Error("lerd-postgres should not be running (not in output)")
	}
}

func TestPollEmptyOutputClearsState(t *testing.T) {
	c := newTestCache(func() (string, error) {
		return "lerd-nginx\trunning", nil
	})
	c.started = true
	c.poll()
	if !c.Running("lerd-nginx") {
		t.Fatal("expected running after first poll")
	}

	// Simulate podman machine stopped: empty output (but no error).
	c.pollFn = func() (string, error) { return "", nil }
	c.poll()
	if c.Running("lerd-nginx") {
		t.Error("should not be running after empty poll (machine stopped)")
	}
}

func TestPollErrorKeepsLastState(t *testing.T) {
	// On error (e.g. VM booting) the cache should go to all-false (safe default).
	calls := 0
	c := newTestCache(func() (string, error) {
		calls++
		if calls == 1 {
			return "lerd-nginx\trunning", nil
		}
		return "", errors.New("podman machine unreachable")
	})
	c.started = true
	c.poll() // first poll: nginx running
	if !c.Running("lerd-nginx") {
		t.Fatal("expected running after first poll")
	}

	c.poll() // second poll: error — fresh map is empty
	if c.Running("lerd-nginx") {
		t.Error("on error the cache should clear (all containers appear stopped)")
	}
}

func TestRunningUsesMapWhenStarted(t *testing.T) {
	// When started=true, Running() must read from the map, never calling
	// ContainerRunning (which would spawn a real podman subprocess).
	pollCalled := false
	c := newTestCache(func() (string, error) {
		pollCalled = true
		return "lerd-nginx\trunning", nil
	})
	c.started = true
	c.poll()

	if !pollCalled {
		t.Fatal("expected poll to be called")
	}
	if !c.Running("lerd-nginx") {
		t.Error("lerd-nginx should be running from cache")
	}
	if c.Running("lerd-notexist") {
		t.Error("unknown container should not be running")
	}
}

func TestSnapshotStartedReturnsCachedMap(t *testing.T) {
	c := newTestCache(func() (string, error) {
		return "lerd-nginx\trunning\nlerd-mysql\texited", nil
	})
	c.started = true
	c.poll()

	snap := c.Snapshot()
	if !snap["lerd-nginx"] {
		t.Error("expected lerd-nginx running in snapshot")
	}
	if snap["lerd-mysql"] {
		t.Error("expected lerd-mysql not running in snapshot")
	}

	// Mutating the returned map must not affect the cache.
	snap["lerd-nginx"] = false
	if !c.Running("lerd-nginx") {
		t.Error("snapshot mutation leaked into cache")
	}
}

func TestSnapshotUnstartedFallsBackToPodman(t *testing.T) {
	calls := 0
	c := newTestCache(func() (string, error) {
		calls++
		return "lerd-nginx\trunning\nlerd-redis\texited", nil
	})
	// Note: c.started left false to exercise the CLI fallback path.

	snap := c.Snapshot()
	if calls != 1 {
		t.Fatalf("expected 1 fallback poll, got %d", calls)
	}
	if !snap["lerd-nginx"] {
		t.Error("expected lerd-nginx in fallback snapshot")
	}
	if snap["lerd-redis"] {
		t.Error("expected lerd-redis not running in fallback snapshot")
	}
}

func TestRefreshTriggersImmediatePoll(t *testing.T) {
	polled := make(chan struct{}, 10)
	c := newTestCache(func() (string, error) {
		polled <- struct{}{}
		return "lerd-nginx\trunning", nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.mu.Lock()
	c.started = true
	c.mu.Unlock()

	// Use a very long interval so the timer doesn't fire during the test.
	c.interval = 10 * time.Minute
	go c.loop(ctx)
	// Drain the initial poll that loop() does not perform (we call poll directly).

	// Signal a refresh.
	c.Refresh()

	select {
	case <-polled:
		// good — poll was triggered
	case <-time.After(2 * time.Second):
		t.Error("Refresh() did not trigger a poll within 2s")
	}
}

func TestSetIntervalTakeEffect(t *testing.T) {
	polled := 0
	var mu sync.Mutex
	c := newTestCache(func() (string, error) {
		mu.Lock()
		polled++
		mu.Unlock()
		return "", nil
	})

	c.mu.Lock()
	c.started = true
	c.mu.Unlock()

	c.interval = 50 * time.Millisecond
	stop := startLoop(t, c)

	time.Sleep(200 * time.Millisecond)
	stop()

	mu.Lock()
	count := polled
	mu.Unlock()

	// With 50ms interval over 200ms we expect ~3-4 polls (not 0, not hundreds).
	if count < 2 || count > 10 {
		t.Errorf("expected 2-10 polls in 200ms with 50ms interval, got %d", count)
	}

	// Now verify SetInterval slows things down.
	mu.Lock()
	polled = 0
	mu.Unlock()
	c.SetInterval(10 * time.Second)
	c.mu.Lock()
	c.started = true
	c.mu.Unlock()
	stop2 := startLoop(t, c)

	time.Sleep(200 * time.Millisecond)
	stop2()

	mu.Lock()
	count2 := polled
	mu.Unlock()
	if count2 > 2 {
		t.Errorf("expected ≤2 polls in 200ms with 10s interval, got %d", count2)
	}
}

func TestConcurrentReads(t *testing.T) {
	c := newTestCache(func() (string, error) {
		return "lerd-nginx\trunning\nlerd-mysql\texited", nil
	})
	c.started = true
	c.poll()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Running("lerd-nginx")
			_ = c.Running("lerd-mysql")
			_ = c.Running("lerd-redis")
		}()
	}
	wg.Wait()
}

func TestConcurrentPollAndRead(t *testing.T) {
	// Ensure no data race between background poll and concurrent reads.
	var callsMu sync.Mutex
	calls := 0
	c := newTestCache(func() (string, error) {
		callsMu.Lock()
		calls++
		n := calls
		callsMu.Unlock()
		if n%2 == 0 {
			return "lerd-nginx\trunning", nil
		}
		return "lerd-mysql\trunning", nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.mu.Lock()
	c.started = true
	c.mu.Unlock()
	c.interval = 10 * time.Millisecond
	go c.loop(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = c.Running("lerd-nginx")
				_ = c.Running("lerd-mysql")
			}
		}()
	}
	wg.Wait()
}

// While lerd is intentionally stopped its containers are meant to be down, and
// workerheal.Detect already suppresses itself, so nothing in the process needs
// fresh container state. The timer must stop spending a podman round trip on
// it, and must pick straight back up once `lerd start` clears the marker.
func TestLoopStopsPollingWhileLerdIsStopped(t *testing.T) {
	var mu sync.Mutex
	polls := 0
	c := newTestCache(func() (string, error) {
		mu.Lock()
		polls++
		mu.Unlock()
		return "", nil
	})

	var stopped atomic.Bool
	stopped.Store(true)
	prev := stoppedFn
	stoppedFn = stopped.Load
	t.Cleanup(func() { stoppedFn = prev })

	c.mu.Lock()
	c.started = true
	c.mu.Unlock()
	c.interval = 20 * time.Millisecond
	startLoop(t, c)

	time.Sleep(250 * time.Millisecond)
	mu.Lock()
	whileStopped := polls
	mu.Unlock()

	// Polling settles once two polls agree. With a pollFn whose answer never
	// changes that is the second one, so anything beyond it is the waste this
	// is meant to remove.
	if whileStopped > 2 {
		t.Errorf("polled %d times while stopped, want no more than the settling polls", whileStopped)
	}

	stopped.Store(false)
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	afterStart := polls
	mu.Unlock()

	if afterStart <= whileStopped {
		t.Errorf("polling did not resume after the stop marker cleared (%d -> %d)", whileStopped, afterStart)
	}
}

// `lerd stop` writes the marker before it tears anything down, so a tick can
// land while containers are still on their way out. Going quiet after that one
// poll would leave the map reporting them as running for the whole stopped
// period, which is worse than the polling this saves.
func TestLoopKeepsPollingUntilTeardownSettles(t *testing.T) {
	var mu sync.Mutex
	polls := 0
	// Mirrors a teardown in progress: still running on the first poll, gone by
	// the second. lerd-dns stays up throughout, as it does in a real stop.
	c := newTestCache(func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		polls++
		if polls == 1 {
			return "lerd-dns\trunning\nlerd-fp-demo\trunning", nil
		}
		return "lerd-dns\trunning\nlerd-fp-demo\texited", nil
	})

	prev := stoppedFn
	stoppedFn = func() bool { return true }
	t.Cleanup(func() { stoppedFn = prev })

	c.mu.Lock()
	c.started = true
	c.mu.Unlock()
	c.interval = 20 * time.Millisecond
	startLoop(t, c)

	time.Sleep(250 * time.Millisecond)

	if c.Running("lerd-fp-demo") {
		t.Error("map still reports a container that went down during the teardown")
	}
	if !c.Running("lerd-dns") {
		t.Error("lerd-dns stays up through a stop and must still read as running")
	}

	mu.Lock()
	got := polls
	mu.Unlock()
	// One poll catching the teardown mid-flight, one seeing it finished, one
	// agreeing with that, then quiet.
	if got != 3 {
		t.Errorf("polled %d times, want the three it takes to settle here", got)
	}
}

// An explicit refresh is a caller saying it needs state now, so it is honoured
// even while stopped. Nothing on the start path uses it today; the loop noticing
// the cleared marker is what resumes polling.
func TestRefreshPollsEvenWhileStopped(t *testing.T) {
	polled := make(chan struct{}, 10)
	c := newTestCache(func() (string, error) {
		polled <- struct{}{}
		return "", nil
	})

	prev := stoppedFn
	stoppedFn = func() bool { return true }
	t.Cleanup(func() { stoppedFn = prev })

	c.mu.Lock()
	c.started = true
	c.mu.Unlock()
	// Long interval so the timer branch never fires: the only poll that can
	// arrive is the one Refresh asks for.
	c.interval = 10 * time.Minute
	startLoop(t, c)

	c.Refresh()

	select {
	case <-polled:
	case <-time.After(2 * time.Second):
		t.Error("explicit Refresh() must poll even while lerd is stopped")
	}
}
