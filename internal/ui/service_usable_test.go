package ui

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

type usableUnits map[string]string

func (u usableUnits) Start(string) error                  { return nil }
func (u usableUnits) Stop(string) error                   { return nil }
func (u usableUnits) Restart(string) error                { return nil }
func (u usableUnits) UnitStatus(n string) (string, error) { return u[n], nil }
func (u usableUnits) AllUnitStates() map[string]string    { return nil }

// A sleeping service's database and bucket actions go through, since the data
// layer wakes it; one the user stopped still asks to be started.
func TestServiceUsable(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	prev := podman.UnitLifecycle
	t.Cleanup(func() { podman.UnitLifecycle = prev })
	podman.UnitLifecycle = usableUnits{"lerd-redis": "active", "lerd-mysql": "inactive", "lerd-postgres": "inactive"}
	_ = config.SetServiceIdleSuspended("mysql", true)

	for name, want := range map[string]bool{"redis": true, "mysql": true, "postgres": false} {
		if got := serviceUsable(name); got != want {
			t.Errorf("serviceUsable(%s) = %v, want %v", name, got, want)
		}
	}
}
