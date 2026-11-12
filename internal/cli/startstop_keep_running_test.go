package cli

import (
	"slices"
	"testing"
)

func TestStartUnitLeavesRunningContainersAlone(t *testing.T) {
	origStart, origRestart, origRunning := startUnitFn, restartUnitFn, containerRunningFn
	t.Cleanup(func() { startUnitFn, restartUnitFn, containerRunningFn = origStart, origRestart, origRunning })

	var started, restarted []string
	startUnitFn = func(name string) error { started = append(started, name); return nil }
	restartUnitFn = func(name string) error { restarted = append(restarted, name); return nil }
	containerRunningFn = func(name string) (bool, error) { return name == "lerd-nginx" || name == "lerd-dns", nil }

	for _, u := range []string{"lerd-nginx", "lerd-redis", "lerd-dns"} {
		if err := startUnit(u); err != nil {
			t.Fatalf("startUnit(%s): %v", u, err)
		}
	}
	if !slices.Equal(started, []string{"lerd-redis"}) {
		t.Errorf("started = %v, want only the stopped lerd-redis", started)
	}
	if !slices.Equal(restarted, []string{"lerd-dns"}) {
		t.Errorf("restarted = %v, want lerd-dns, which always restarts", restarted)
	}
}
