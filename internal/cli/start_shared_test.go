package cli

import (
	"slices"
	"testing"
)

// lerd start and lerd install start a service through the start every other
// caller of that service shares; core units and workers start as before.
func TestStartUnitShared_routesOnlyServices(t *testing.T) {
	var shared, direct []string
	prevShared, prevStart := startSharedFn, startOneUnit
	startSharedFn = func(name string, start func() error) error { shared = append(shared, name); return start() }
	startOneUnit = func(unit string) error { direct = append(direct, unit); return nil }
	t.Cleanup(func() { startSharedFn, startOneUnit = prevShared, prevStart })

	services := map[string]bool{"lerd-mysql": true, "lerd-redis": true}
	for _, u := range []string{"lerd-mysql", "lerd-nginx", "lerd-queue-shop", "lerd-redis"} {
		if err := startUnitShared(u, services); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(shared, []string{"mysql", "redis"}) {
		t.Errorf("shared starts = %v, want the two services", shared)
	}
	if !slices.Equal(direct, []string{"lerd-mysql", "lerd-nginx", "lerd-queue-shop", "lerd-redis"}) {
		t.Errorf("units started = %v, want every unit started once", direct)
	}
}
