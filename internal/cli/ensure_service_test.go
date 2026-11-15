//go:build linux

package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/podman"
)

// inactiveLifecycle reports every unit as inactive so ensureServiceRunning walks
// past the already-active shortcut and into the preset/custom resolution branch.
type inactiveLifecycle struct{}

func (inactiveLifecycle) Start(string) error                { return nil }
func (inactiveLifecycle) Stop(string) error                 { return nil }
func (inactiveLifecycle) Restart(string) error              { return nil }
func (inactiveLifecycle) UnitStatus(string) (string, error) { return "inactive", nil }
func (inactiveLifecycle) AllUnitStates() map[string]string  { return nil }

func stubInactiveUnits(t *testing.T) {
	t.Helper()
	prev := podman.UnitLifecycle
	podman.UnitLifecycle = inactiveLifecycle{}
	t.Cleanup(func() { podman.UnitLifecycle = prev })
}

// A .lerd.yaml that names a bundled preset which was never installed must point
// the user at the install command, not at a missing custom-service YAML.
func TestEnsureServiceRunning_pointsAtPresetWhenOneExists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubInactiveUnits(t)

	err := ensureServiceRunning("phpmyadmin")
	if err == nil {
		t.Fatal("expected an error for an uninstalled preset")
	}
	if !strings.Contains(err.Error(), "lerd service preset phpmyadmin") {
		t.Errorf("error should point at the preset install command, got: %v", err)
	}
	if strings.Contains(err.Error(), "no such file") {
		t.Errorf("error should not mention a missing custom file, got: %v", err)
	}
}

// A name with neither a preset nor a custom YAML keeps the custom-service error.
func TestEnsureServiceRunning_keepsCustomErrorWhenNoPreset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubInactiveUnits(t)

	err := ensureServiceRunning("totallycustom")
	if err == nil {
		t.Fatal("expected an error for an unknown service")
	}
	if !strings.Contains(err.Error(), "custom service \"totallycustom\" not found") {
		t.Errorf("error should reference the missing custom service, got: %v", err)
	}
}

// stubWatcher stands in for the watcher's answer and records local starts.
func stubWatcher(t *testing.T, handled bool, err error, starting bool) *[]string {
	t.Helper()
	var local []string
	prevAsk, prevLocal := askWatcherToEnsure, localEnsureService
	askWatcherToEnsure = func(name string, onStarting func()) (bool, error) {
		if starting && onStarting != nil {
			onStarting()
		}
		return handled, err
	}
	localEnsureService = func(name string) error { local = append(local, name); return nil }
	t.Cleanup(func() { askWatcherToEnsure, localEnsureService = prevAsk, prevLocal })
	return &local
}

// Once the watcher answers, its answer stands: this process starts nothing.
func TestEnsureServiceRunning_takesTheWatchersAnswer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubInactiveUnits(t)

	local := stubWatcher(t, true, nil, false)
	if err := ensureServiceRunning("mysql"); err != nil || len(*local) != 0 {
		t.Fatalf("err=%v local=%v, want the watcher's ok and no local start", err, *local)
	}
	local = stubWatcher(t, true, errors.New("mysql would not start"), false)
	if err := ensureServiceRunning("mysql"); err == nil || err.Error() != "mysql would not start" || len(*local) != 0 {
		t.Fatalf("err=%v local=%v, want the watcher's error and no local start", err, *local)
	}
}

// Without a watcher to answer, the process brings the service up itself.
func TestEnsureServiceRunning_startsItselfWhenNoWatcherAnswers(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubInactiveUnits(t)

	local := stubWatcher(t, false, nil, false)
	if err := ensureServiceRunning("mysql"); err != nil || len(*local) != 1 {
		t.Fatalf("err=%v local=%v, want one local start", err, *local)
	}
}

// The watcher saying it has to start the service shows the same line as before.
func TestEnsureServiceRunning_saysStartingWhenTheWatcherDoes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubInactiveUnits(t)
	stubWatcher(t, true, nil, true)

	r, w, _ := os.Pipe()
	prev := os.Stderr
	os.Stderr = w
	err := ensureServiceRunning("mysql")
	os.Stderr = prev
	w.Close()
	out, _ := io.ReadAll(r)
	if err != nil || !strings.Contains(string(out), "Starting mysql...") {
		t.Fatalf("err=%v stderr=%q, want the Starting line", err, out)
	}
}
