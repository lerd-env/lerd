package cli

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/services"
)

// devPortMgr answers IsActive for the units named in active and nothing else.
type devPortMgr struct {
	services.ServiceManager
	active map[string]bool
}

func (m devPortMgr) IsActive(name string) bool { return m.active[name] }

func swapDevPortMgr(t *testing.T, m services.ServiceManager) {
	t.Helper()
	prev := services.Mgr
	services.Mgr = m
	t.Cleanup(func() { services.Mgr = prev })
}

// heldPort binds a port for the duration of the test and returns it, so the
// pin under test is genuinely unbindable rather than merely claimed.
func heldPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

func siteWithPin(t *testing.T, dir string, port int) string {
	t.Helper()
	path := filepath.Join(dir, "myapp")
	if err := config.AddSite(config.Site{
		Name:          "myapp",
		Path:          path,
		Domains:       []string{"myapp.test"},
		DevServerPort: port,
	}); err != nil {
		t.Fatal(err)
	}
	return path
}

// A dev server running on its pinned port makes that port unbindable, which
// used to read as taken by another process: the pin moved, the vhost followed
// it, and the tool kept serving the port it started on until it was restarted.
func TestDevServerEnsurePortKeepsAPinItsOwnDevServerHolds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	pinned := heldPort(t)
	path := siteWithPin(t, dir, pinned)
	swapDevPortMgr(t, devPortMgr{active: map[string]bool{"lerd-vite-myapp": true}})

	got, err := devServerEnsurePort("myapp", path, "vite", 5173)
	if err != nil {
		t.Fatal(err)
	}
	if got != pinned {
		t.Fatalf("devServerEnsurePort() = %d, want it to keep %d, the port its own dev server is serving", got, pinned)
	}
}

// A pin held by anything other than the site's dev server still has to be
// re-picked, or the tool is launched with strict-port against a port it cannot
// bind and refuses to start at all.
func TestDevServerEnsurePortRepicksAPinHeldByAnotherProcess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	pinned := heldPort(t)
	path := siteWithPin(t, dir, pinned)
	swapDevPortMgr(t, devPortMgr{active: map[string]bool{}})

	got, err := devServerEnsurePort("myapp", path, "vite", 5173)
	if err != nil {
		t.Fatal(err)
	}
	if got == pinned {
		t.Fatalf("devServerEnsurePort() kept %d, a port nothing of ours is holding and nothing can bind", pinned)
	}
}
