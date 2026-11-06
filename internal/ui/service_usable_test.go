package ui

import (
	"net"
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

// An external server has no unit: it is usable while it answers.
func TestServiceUsable_ExternalServerGoesByWhetherItAnswers(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	svc := &config.CustomService{Name: "dbngin", Family: "mysql", External: &config.ExternalDB{Host: "host.containers.internal", Port: ln.Addr().(*net.TCPAddr).Port}}
	if err := config.SaveCustomService(svc); err != nil {
		t.Fatal(err)
	}
	if !serviceUsable("dbngin") {
		t.Error("a server that answers is usable")
	}
	ln.Close()
	if serviceUsable("dbngin") {
		t.Error("a server that stopped answering is not")
	}
}
