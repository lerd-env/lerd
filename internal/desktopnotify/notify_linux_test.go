//go:build linux

package desktopnotify

import "testing"

// resetRouteState clears the package-level click-route tracking between cases.
func resetRouteState(t *testing.T) {
	t.Helper()
	routeMu.Lock()
	routes = map[uint32]string{}
	routeOrder = nil
	listenerActive = false
	routeMu.Unlock()
	t.Cleanup(func() {
		routeMu.Lock()
		routes = map[uint32]string{}
		routeOrder = nil
		listenerActive = false
		routeMu.Unlock()
	})
}

func withListener(t *testing.T, ok bool, calls *int) {
	t.Helper()
	orig := startActionListener
	startActionListener = func() bool {
		if calls != nil {
			*calls++
		}
		return ok
	}
	t.Cleanup(func() { startActionListener = orig })
}

// A listener that never started cannot deliver clicks, so tracking a route
// would grow the map forever with entries nothing will ever consume.
func TestTrackRouteDropsWhenListenerFails(t *testing.T) {
	resetRouteState(t)
	calls := 0
	withListener(t, false, &calls)

	trackRoute(1, "#system")
	trackRoute(2, "#sites")

	routeMu.Lock()
	n := len(routes)
	routeMu.Unlock()
	if n != 0 {
		t.Fatalf("tracked %d routes with no listener, want 0", n)
	}
	if calls != 2 {
		t.Fatalf("listener start attempted %d times, want 2 (a failure must not be latched)", calls)
	}
}

func TestTrackRouteStartsListenerOnce(t *testing.T) {
	resetRouteState(t)
	calls := 0
	withListener(t, true, &calls)

	trackRoute(1, "#system")
	trackRoute(2, "#sites")

	if calls != 1 {
		t.Fatalf("listener started %d times, want 1", calls)
	}
	routeMu.Lock()
	n := len(routes)
	routeMu.Unlock()
	if n != 2 {
		t.Fatalf("tracked %d routes, want 2", n)
	}
}

// Popups the user never clicks and whose daemon sends no close signal would
// otherwise accumulate for the life of the daemon.
func TestTrackRouteEvictsOldest(t *testing.T) {
	resetRouteState(t)
	withListener(t, true, nil)

	for i := 0; i < maxTrackedRoutes+50; i++ {
		trackRoute(uint32(i), "#system")
	}

	routeMu.Lock()
	n := len(routes)
	_, oldestKept := routes[0]
	_, newestKept := routes[uint32(maxTrackedRoutes+49)]
	routeMu.Unlock()

	if n > maxTrackedRoutes {
		t.Fatalf("routes grew to %d, want at most %d", n, maxTrackedRoutes)
	}
	if oldestKept {
		t.Error("oldest route survived eviction")
	}
	if !newestKept {
		t.Error("newest route was evicted")
	}
}
