//go:build linux

package cli

import (
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
