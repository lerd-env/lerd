package serviceops

import (
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
