package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/services"
)

// trayUnitMgr records what the heal did to the tray unit, and answers whether
// the unit is armed.
type trayUnitMgr struct {
	fakeServiceMgr
	enabled  bool
	disabled []string
	stopped  []string
}

func (m *trayUnitMgr) IsEnabled(string) bool { return m.enabled }
func (m *trayUnitMgr) Disable(name string) error {
	m.disabled = append(m.disabled, name)
	m.enabled = false
	return nil
}
func (m *trayUnitMgr) Stop(name string) error {
	m.stopped = append(m.stopped, name)
	return nil
}

// writeTrayPreference lays down a config with the tray preference set, in an
// isolated config dir so the test never reads the machine's own.
func writeTrayPreference(t *testing.T, disabled bool) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := config.ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "tray:\n    disabled: false\n"
	if disabled {
		body = "tray:\n    disabled: true\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// The desktop session starts an armed unit at login without consulting the
// preference, so a unit still armed after the tray was turned off brings it
// back on every boot.
func TestHealArmedTrayUnit_disarmsAUnitLeftArmed(t *testing.T) {
	writeTrayPreference(t, true)
	m := &trayUnitMgr{enabled: true}
	swapMgr(t, m)

	if !healArmedTrayUnit() {
		t.Fatal("healArmedTrayUnit() = false, want the drift reported")
	}
	if len(m.disabled) != 1 || m.disabled[0] != "lerd-tray" {
		t.Errorf("disabled = %v, want the tray unit disarmed once", m.disabled)
	}
}

func TestHealArmedTrayUnit_leavesAnArmedUnitAloneWhenTheTrayIsOn(t *testing.T) {
	writeTrayPreference(t, false)
	m := &trayUnitMgr{enabled: true}
	swapMgr(t, m)

	if healArmedTrayUnit() {
		t.Error("healArmedTrayUnit() = true, want the tray left armed while it is switched on")
	}
	if len(m.disabled) != 0 {
		t.Errorf("disabled = %v, want nothing touched", m.disabled)
	}
}

// The ordinary machine: preference off and no unit armed. The heal has to stay
// quiet rather than report a repair on every start.
func TestHealArmedTrayUnit_quietWhenNothingIsArmed(t *testing.T) {
	writeTrayPreference(t, true)
	m := &trayUnitMgr{enabled: false}
	swapMgr(t, m)

	if healArmedTrayUnit() {
		t.Error("healArmedTrayUnit() = true, want silence when no unit is armed")
	}
	if len(m.disabled) != 0 {
		t.Errorf("disabled = %v, want nothing touched", m.disabled)
	}
}

var _ services.ServiceManager = (*trayUnitMgr)(nil)
