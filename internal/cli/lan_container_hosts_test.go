package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/services"
)

// Restarting lerd-nginx gives it a new bridge address, and the hosts file
// bind-mounted into every PHP-FPM container still points .test domains at the
// old one. Without a refresh here, container-side resolution stays dead until
// the watcher's next inspect.
func TestRegenerateLANQuadletsRefreshesContainerHosts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveGlobal(&config.GlobalConfig{}); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	writeServiceQuadlet(t, "lerd-nginx", "PublishPort=127.0.0.1:80:80")

	prevProbe := podman.ContainerPublishesLANFn
	podman.ContainerPublishesLANFn = func(name string) (bool, bool) { return name == "lerd-nginx", true }
	t.Cleanup(func() { podman.ContainerPublishesLANFn = prevProbe })

	prevMgr := services.Mgr
	services.Mgr = &rebindMgr{}
	t.Cleanup(func() { services.Mgr = prevMgr })

	writes := 0
	prevWrite := writeContainerHostsFn
	writeContainerHostsFn = func() error { writes++; return nil }
	t.Cleanup(func() { writeContainerHostsFn = prevWrite })

	if err := regenerateLANContainerQuadlets(nil); err != nil {
		t.Fatalf("regenerateLANContainerQuadlets: %v", err)
	}
	if writes != 1 {
		t.Errorf("container hosts written %d times, want 1", writes)
	}
}

// inactiveMgr reports every unit stopped, so the rebind rewrites files and
// restarts nothing.
type inactiveMgr struct{ services.ServiceManager }

func (inactiveMgr) DaemonReload() error { return nil }

func (inactiveMgr) UnitStatus(string) (string, error) { return "inactive", nil }

// With nothing restarted, no address moved, so the refresh (a podman probe) is
// not worth paying for.
func TestRegenerateLANQuadletsSkipsHostsRefreshWhenNothingRestarts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveGlobal(&config.GlobalConfig{}); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	writeServiceQuadlet(t, "lerd-redis", "PublishPort=127.0.0.1:6379:6379")

	prevProbe := podman.ContainerPublishesLANFn
	podman.ContainerPublishesLANFn = func(string) (bool, bool) { return false, true }
	t.Cleanup(func() { podman.ContainerPublishesLANFn = prevProbe })

	prevMgr := services.Mgr
	services.Mgr = inactiveMgr{}
	t.Cleanup(func() { services.Mgr = prevMgr })

	writes := 0
	prevWrite := writeContainerHostsFn
	writeContainerHostsFn = func() error { writes++; return nil }
	t.Cleanup(func() { writeContainerHostsFn = prevWrite })

	if err := regenerateLANContainerQuadlets(nil); err != nil {
		t.Fatalf("regenerateLANContainerQuadlets: %v", err)
	}
	if writes != 0 {
		t.Errorf("container hosts written %d times, want 0", writes)
	}
}
