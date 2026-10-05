package serviceops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

type noopLifecycle struct{}

func (noopLifecycle) Start(string) error                { return nil }
func (noopLifecycle) Stop(string) error                 { return nil }
func (noopLifecycle) Restart(string) error              { return nil }
func (noopLifecycle) UnitStatus(string) (string, error) { return "inactive", nil }
func (noopLifecycle) AllUnitStates() map[string]string  { return nil }

func stubLifecycle(t *testing.T) {
	t.Helper()
	prev := podman.UnitLifecycle
	podman.UnitLifecycle = noopLifecycle{}
	t.Cleanup(func() { podman.UnitLifecycle = prev })

	prevWait := waitReadyFn
	waitReadyFn = func(string, time.Duration) error { return nil }
	t.Cleanup(func() { waitReadyFn = prevWait })

	stubDaemonReload(t)

	prevExists, prevSize, prevPull := imageExistsFn, imageSizeFn, pullImageFn
	imageExistsFn = func(string) bool { return true }
	imageSizeFn = func(string) (int64, bool) { return 0, false }
	pullImageFn = func(string) error { return nil }
	t.Cleanup(func() { imageExistsFn, imageSizeFn, pullImageFn = prevExists, prevSize, prevPull })
}

func TestStartService_unknown(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	err := StartService("nosuch-service")
	if err == nil || !strings.Contains(err.Error(), "unknown service") {
		t.Fatalf("StartService unknown = %v", err)
	}
}

func TestStartService_customStartsAndMarksManual(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	prevRun := config.ServiceRunning
	config.ServiceRunning = func(string) bool { return false }
	t.Cleanup(func() { config.ServiceRunning = prevRun })

	if err := config.SaveCustomService(&config.CustomService{
		Name: "mailhog-test", Image: "docker.io/library/alpine:latest",
		Ports: []string{"127.0.0.1:1025:1025"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := StartService("mailhog-test"); err != nil {
		t.Fatalf("StartService: %v", err)
	}
	if !config.ServiceIsManuallyStarted("mailhog-test") {
		t.Fatal("StartService must mark the service manually started")
	}
	if config.ServiceIsPaused("mailhog-test") {
		t.Fatal("StartService must clear paused")
	}
}

func TestStopService_marksPaused(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mailhog-test", Image: "docker.io/library/alpine:latest",
		Ports: []string{"127.0.0.1:1025:1025"},
	}); err != nil {
		t.Fatal(err)
	}
	_ = config.SetServiceManuallyStarted("mailhog-test", true)
	if err := StopService("mailhog-test"); err != nil {
		t.Fatalf("StopService: %v", err)
	}
	if !config.ServiceIsPaused("mailhog-test") {
		t.Fatal("StopService must mark paused")
	}
	if config.ServiceIsManuallyStarted("mailhog-test") {
		t.Fatal("StopService must clear manually started")
	}
}

func TestRestartService_clearsPaused(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mailhog-test", Image: "docker.io/library/alpine:latest",
		Ports: []string{"127.0.0.1:1025:1025"},
	}); err != nil {
		t.Fatal(err)
	}
	_ = config.SetServicePaused("mailhog-test", true)
	if err := RestartService("mailhog-test"); err != nil {
		t.Fatalf("RestartService: %v", err)
	}
	if config.ServiceIsPaused("mailhog-test") {
		t.Fatal("RestartService must clear paused")
	}
	if !config.ServiceIsManuallyStarted("mailhog-test") {
		t.Fatal("RestartService must mark manually started")
	}
}

type failStopLifecycle struct{ noopLifecycle }

func (failStopLifecycle) Stop(string) error { return errors.New("stop refused") }
func (failStopLifecycle) UnitStatus(string) (string, error) {
	return "active", nil
}

func TestStopWithDependents_returnsStopError(t *testing.T) {
	withServiceHome(t)
	prev := podman.UnitLifecycle
	podman.UnitLifecycle = failStopLifecycle{}
	t.Cleanup(func() { podman.UnitLifecycle = prev })

	err := StopWithDependents("redis")
	if err == nil || !strings.Contains(err.Error(), "stop refused") {
		t.Fatalf("StopWithDependents = %v, want stop refused", err)
	}
}

// Dependents are matched by family and env_role, not just by literal name, so a
// service whose own family satisfies one of its own depends_on entries appears
// in its own dependent list. The cascade has to notice.
func TestStopWithDependentsTerminatesOnASelfSatisfyingService(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "lerd", "services")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "name: weird\nimage: example/x:1\nfamily: mysql\ndepends_on:\n  - mysql\n"
	if err := os.WriteFile(filepath.Join(dir, "weird.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// The precondition the guard exists for.
	if deps := dependentsOf("weird"); len(deps) != 1 || deps[0] != "weird" {
		t.Fatalf("dependentsOf(weird) = %v, want [weird]", deps)
	}
	if !dependentNeedsCascade("weird", "weird") {
		t.Fatal("dependentNeedsCascade(weird, weird) = false; the loop is no longer reachable this way")
	}

	done := make(chan error, 1)
	go func() { done <- StopWithDependents("weird") }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("StopWithDependents did not terminate on a self-satisfying service")
	}
}

// A first start that cannot fetch its image must not leave the service
// installed behind it, restart-looping on a pull that has already failed.
func TestStartService_builtinPullFailureLeavesNothingInstalled(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	imageExistsFn = func(string) bool { return false }
	pullImageFn = func(string) error { return errors.New("network is unreachable") }

	if err := StartService("mailpit"); err == nil {
		t.Fatal("a failed pull must fail the start")
	}
	if _, err := os.Stat(filepath.Join(config.QuadletDir(), "lerd-mailpit.container")); err == nil {
		t.Fatal("the failed first start left a mailpit unit installed")
	}
}

// The image is fetched, announced, before the unit starts, so podman never
// pulls it silently in the middle of the start.
func TestStartService_pullsAMissingImageBeforeStarting(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	imageExistsFn = func(string) bool { return false }
	var pulled []string
	pullImageFn = func(img string) error { pulled = append(pulled, img); return nil }

	if err := StartService("mailpit"); err != nil {
		t.Fatalf("StartService: %v", err)
	}
	if len(pulled) != 1 || !strings.Contains(pulled[0], "mailpit") {
		t.Fatalf("expected the mailpit image pulled once before the start, got %v", pulled)
	}
}

// A built-in with no unit yet still names the image its first start fetches,
// which is what the CLI, dashboard and MCP disclose before downloading.
func TestStartImage_namesABuiltinsImageBeforeItIsInstalled(t *testing.T) {
	withServiceHome(t)
	if img := StartImage("mailpit"); !strings.Contains(img, "mailpit") {
		t.Fatalf("StartImage(mailpit) = %q", img)
	}
}

// A service the user stopped and a site then needs is running again, so the
// pause must go with it; left behind, the next lerd start skips the service
// while the site's .env still points at it.
func TestEnsureServiceRunning_clearsThePauseOfAServiceItStarts(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mailhog-test", Image: "docker.io/library/alpine:latest",
		Ports: []string{"127.0.0.1:1025:1025"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := StopService("mailhog-test"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureServiceRunning("mailhog-test"); err != nil {
		t.Fatalf("EnsureServiceRunning: %v", err)
	}
	if config.ServiceIsPaused("mailhog-test") {
		t.Fatal("a service started for a site is still recorded as paused")
	}
}

type activeLifecycle struct{ noopLifecycle }

func (activeLifecycle) UnitStatus(string) (string, error) { return "active", nil }

// An install that already ran a paused service, from before the pause was
// cleared on start, heals the next time a site asks for it.
func TestEnsureServiceRunning_clearsThePauseOfAServiceAlreadyRunning(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	podman.UnitLifecycle = activeLifecycle{}
	_ = config.SetServicePaused("mailhog-test", true)
	if err := EnsureServiceRunning("mailhog-test"); err != nil {
		t.Fatalf("EnsureServiceRunning: %v", err)
	}
	if config.ServiceIsPaused("mailhog-test") {
		t.Fatal("a running service is still recorded as paused")
	}
}
