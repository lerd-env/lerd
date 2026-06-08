package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A host-proxy site whose .lerd.yaml dev command has drifted from the approved
// one must still be enumerated, because this list also drives lerd stop/quit:
// excluding the unit would leave a running dev server unstoppable.
func TestRegisteredFrameworkWorkerUnits_EnumeratesDriftedHostProxy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := config.SaveProjectConfig(dir, &config.ProjectConfig{
		Proxy: &config.ProxyConfig{Command: "npm run drifted", Port: 5173},
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name: "site", Domains: []string{"site.test"}, Path: dir,
		HostPort: 5173, HostCommand: "npm run approved",
	}); err != nil {
		t.Fatal(err)
	}
	want := config.HostProxyWorkerUnit("site")
	found := false
	for _, u := range registeredFrameworkWorkerUnits() {
		if u == want {
			found = true
		}
	}
	if !found {
		t.Errorf("drifted host-proxy unit %q must stay enumerated so stop/quit can stop it", want)
	}
}

func TestQuadletImage_found(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "containers", "systemd")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "lerd-nginx.container"), []byte("[Container]\nImage=docker.io/library/nginx:alpine\n"), 0644)

	got := quadletImage("lerd-nginx")
	if got != "docker.io/library/nginx:alpine" {
		t.Errorf("quadletImage = %q, want docker.io/library/nginx:alpine", got)
	}
}

func TestQuadletImage_missing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got := quadletImage("lerd-nonexistent")
	if got != "" {
		t.Errorf("quadletImage = %q, want empty for missing unit", got)
	}
}

func TestQuadletImage_noImageLine(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "containers", "systemd")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "lerd-test.container"), []byte("[Container]\nContainerName=test\n"), 0644)

	got := quadletImage("lerd-test")
	if got != "" {
		t.Errorf("quadletImage = %q, want empty when no Image= line", got)
	}
}

func TestIsPortConflict(t *testing.T) {
	const portList = "dnsmasq 60498 sdp 5u IPv4 TCP 127.0.0.1:5300 (LISTEN)"
	dnsCheck := PortCheck{Port: "5300", Label: "dns", Container: "lerd-dns"}
	mariadb := PortCheck{Port: "3306", Label: "mariadb", Container: "lerd-mariadb"}

	running := func(string) bool { return true }
	notRunning := func(string) bool { return false }
	dnsUp := func() bool { return true }
	dnsDown := func() bool { return false }

	tests := []struct {
		name             string
		check            PortCheck
		ports            string
		containerRunning func(string) bool
		dnsAnswering     func() bool
		want             bool
	}{
		// The regression this fixes: lerd-dns is a launchd dnsmasq (no
		// container), already answering, holding 5300 — NOT a conflict.
		{"dns self-owns port via launchd dnsmasq", dnsCheck, portList, notRunning, dnsUp, false},
		// dns genuinely down but something foreign holds 5300 — real conflict.
		{"dns down with foreign listener on 5300", dnsCheck, portList, notRunning, dnsDown, true},
		// A running container owns its port directly — never a conflict.
		{"running container owns its port", mariadb, "mysqld 1 sdp 3u TCP 127.0.0.1:3306 (LISTEN)", running, dnsDown, false},
		// Non-dns service, not running, foreign listener — real conflict.
		{"foreign process holds a service port", mariadb, "someapp 999 sdp 3u TCP 127.0.0.1:3306 (LISTEN)", notRunning, dnsDown, true},
		// The macOS regression: gvproxy (podman machine's own forwarder) holds
		// the published port — a lerd forward into the VM, NOT a conflict.
		{"gvproxy forward owns a service port", mariadb, "gvproxy 82853 sdp 12u TCP 127.0.0.1:3306 (LISTEN)", notRunning, dnsDown, false},
		// Port is free — no conflict regardless.
		{"port free", mariadb, portList, notRunning, dnsDown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPortConflict(tt.check, tt.ports, tt.containerRunning, tt.dnsAnswering)
			if got != tt.want {
				t.Errorf("isPortConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnsurePodmanMachineRunning_linux(t *testing.T) {
	// On Linux this is a no-op — should not panic
	ensurePodmanMachineRunning()
}

func TestMigrateExecWorkerPlists_linux(t *testing.T) {
	// On Linux this is a no-op — should not panic
	migrateExecWorkerPlists()
}
