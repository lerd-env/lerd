package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// TestSetSiteContainerAutostart_stripAndRestore guards the fix for a paused
// FrankenPHP site whose container came back at boot: pausing must strip the
// quadlet's [Install] so the podman generator stops auto-wiring it into
// default.target.wants, and unpausing must put it back.
func TestSetSiteContainerAutostart_stripAndRestore(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // QuadletDir + global config live here

	site := &config.Site{Name: "personal", Runtime: "frankenphp"}
	unit := podman.FrankenPHPContainerName(site.Name)
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir quadlet dir: %v", err)
	}
	path := filepath.Join(dir, unit+".container")
	const withInstall = "[Container]\nImage=x\n\n[Install]\nWantedBy=default.target\n"
	if err := os.WriteFile(path, []byte(withInstall), 0644); err != nil {
		t.Fatalf("seed quadlet: %v", err)
	}

	// Pause strips [Install].
	if !setSiteContainerAutostart(site, false) {
		t.Fatal("strip reported no change")
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "[Install]") {
		t.Errorf("[Install] not stripped:\n%s", b)
	}

	// Unpause restores it (no config file => global autostart enabled by default).
	if !setSiteContainerAutostart(site, true) {
		t.Fatal("restore reported no change")
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "[Install]") || !strings.Contains(string(b), "WantedBy=default.target") {
		t.Errorf("[Install] not restored:\n%s", b)
	}
}

// TestRefreshHostProxyEnvOnResume covers the unpause env-follow: a host-proxy
// site's .env must be regenerated on resume (a port move that landed while it was
// paused was skipped by the move-time follow), while a container site is left
// alone — it reaches services by name on the unchanged internal port.
func TestRefreshHostProxyEnvOnResume(t *testing.T) {
	saved := hostProxyEnvRefresh
	t.Cleanup(func() { hostProxyEnvRefresh = saved })

	var refreshed []string
	hostProxyEnvRefresh = func(dir string) error {
		refreshed = append(refreshed, dir)
		return nil
	}

	// Container site (no HostPort): no refresh.
	container := &config.Site{Name: "shop", Path: "/p/shop"}
	if refreshHostProxyEnvOnResume(container) {
		t.Error("container site should not trigger an env refresh on resume")
	}

	// Host-proxy site (HostPort > 0): refresh its path.
	proxy := &config.Site{Name: "api", Path: "/p/api", HostPort: 3100}
	if !refreshHostProxyEnvOnResume(proxy) {
		t.Error("host-proxy site should trigger an env refresh on resume")
	}
	if len(refreshed) != 1 || refreshed[0] != "/p/api" {
		t.Errorf("refreshed = %v, want exactly [/p/api]", refreshed)
	}
}

// TestSetSiteContainerAutostart_plainFPMNoop confirms a plain FPM site (no
// dedicated container quadlet) is a no-op, so pause/unpause never touch the
// shared FPM container's autostart.
func TestSetSiteContainerAutostart_plainFPMNoop(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	site := &config.Site{Name: "plain"} // no runtime, no custom container
	if setSiteContainerAutostart(site, false) {
		t.Error("plain FPM site should have no container quadlet to strip")
	}
}
