package tools

import (
	"strings"
	"testing"
)

// lerd installs Podman itself on Windows, from the per-user MSI Podman
// publishes, verified against a pinned digest like every other download.
func TestPodmanIsPinnedForWindows(t *testing.T) {
	m := embeddedManifest()
	for _, arch := range []string{"amd64", "arm64"} {
		url, err := m.URL("podman", "windows", arch)
		if err != nil {
			t.Fatalf("podman has no windows/%s asset: %v", arch, err)
		}
		if !strings.HasSuffix(url, "/podman-installer-windows-"+arch+".msi") {
			t.Errorf("windows/%s url = %s, want the MSI installer", arch, url)
		}
		if !strings.Contains(url, m.Tools["podman"].Version) {
			t.Errorf("windows/%s url %s does not pin version %s", arch, url, m.Tools["podman"].Version)
		}
		if m.Digest("podman", "windows", arch) == "" {
			t.Errorf("windows/%s has no digest to verify the installer against", arch)
		}
	}
}
