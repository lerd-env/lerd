package cli

import (
	"io"
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/podman"
)

// A version whose image ensureFPMQuadletTo just rebuilt must have its container
// bounced onto it. StartUnit is a no-op on an active unit, so before this the
// deferred half of a php:ext / php:pkg change left the container serving the
// image it had superseded while every status surface reported the new set.
func TestEnsureFPMQuadletTo_restartsOnlyWhenTheImageWasRebuilt(t *testing.T) {
	origBuild, origStart, origRestart := buildFPMImageTo, startUnitFn, restartUnitFn
	origWrite := podman.WriteContainerUnitFn
	t.Cleanup(func() {
		buildFPMImageTo, startUnitFn, restartUnitFn = origBuild, origStart, origRestart
		podman.WriteContainerUnitFn = origWrite
	})
	podman.WriteContainerUnitFn = func(_, _ string) error { return nil }

	var started, restarted []string
	startUnitFn = func(name string) error { started = append(started, name); return nil }
	restartUnitFn = func(name string) error { restarted = append(restarted, name); return nil }

	rebuilt := false
	buildFPMImageTo = func(_ string, _ bool, _ io.Writer) (bool, error) { return rebuilt, nil }

	if err := ensureFPMQuadletTo("8.3", io.Discard); err != nil {
		t.Fatalf("ensureFPMQuadletTo: %v", err)
	}
	if len(restarted) != 0 {
		t.Errorf("an unchanged image must not bounce a running container, restarted %v", restarted)
	}
	if len(started) != 1 || started[0] != "lerd-php83-fpm" {
		t.Errorf("started = %v, want [lerd-php83-fpm]", started)
	}

	rebuilt = true
	started, restarted = nil, nil
	if err := ensureFPMQuadletTo("8.3", io.Discard); err != nil {
		t.Fatalf("ensureFPMQuadletTo: %v", err)
	}
	if len(started) != 0 {
		t.Errorf("a rebuilt image must not be left to a no-op start, started %v", started)
	}
	if len(restarted) != 1 || restarted[0] != "lerd-php83-fpm" {
		t.Errorf("restarted = %v, want [lerd-php83-fpm]", restarted)
	}
}

// A build that fails never reaches the unit at all.
func TestEnsureFPMQuadletTo_failedBuildTouchesNoUnit(t *testing.T) {
	origBuild, origStart, origRestart := buildFPMImageTo, startUnitFn, restartUnitFn
	origWrite := podman.WriteContainerUnitFn
	t.Cleanup(func() {
		buildFPMImageTo, startUnitFn, restartUnitFn = origBuild, origStart, origRestart
		podman.WriteContainerUnitFn = origWrite
	})
	podman.WriteContainerUnitFn = func(_, _ string) error { return nil }

	touched := 0
	startUnitFn = func(string) error { touched++; return nil }
	restartUnitFn = func(string) error { touched++; return nil }
	buildFPMImageTo = func(_ string, _ bool, _ io.Writer) (bool, error) {
		return false, io.ErrUnexpectedEOF
	}

	if err := ensureFPMQuadletTo("8.3", io.Discard); err == nil {
		t.Fatal("a failed build must surface an error")
	}
	if touched != 0 {
		t.Errorf("a failed build must not start or restart the unit, touched %d times", touched)
	}
}

// fetch rebuilds a stale image without going near the unit, so it needs the
// same bounce ensureFPMQuadletTo does. It must not start anything that was
// down: pre-building images is not a reason to bring a version up.
func TestRestartRebuiltFPMUnits_bouncesOnlyRunningRebuiltVersions(t *testing.T) {
	origRunning, origRestart := fpmContainerRunning, restartUnitFn
	t.Cleanup(func() { fpmContainerRunning, restartUnitFn = origRunning, origRestart })

	up := map[string]bool{"lerd-php83-fpm": true, "lerd-php85-fpm": true}
	fpmContainerRunning = func(name string) (bool, error) { return up[name], nil }

	var restarted []string
	restartUnitFn = func(name string) error { restarted = append(restarted, name); return nil }

	// 8.4 was rebuilt but is down; 8.2 is down and was not rebuilt either.
	restartRebuiltFPMUnits([]string{"8.3", "8.4"})

	if !reflect.DeepEqual(restarted, []string{"lerd-php83-fpm"}) {
		t.Errorf("restarted = %v, want [lerd-php83-fpm]: only a running version this run rebuilt", restarted)
	}
}

// A run that rebuilt nothing must leave every container alone.
func TestRestartRebuiltFPMUnits_nothingRebuiltBouncesNothing(t *testing.T) {
	origRunning, origRestart := fpmContainerRunning, restartUnitFn
	t.Cleanup(func() { fpmContainerRunning, restartUnitFn = origRunning, origRestart })

	fpmContainerRunning = func(string) (bool, error) { return true, nil }
	restarted := 0
	restartUnitFn = func(string) error { restarted++; return nil }

	restartRebuiltFPMUnits(nil)

	if restarted != 0 {
		t.Errorf("restarted %d units for a fetch that rebuilt nothing", restarted)
	}
}
