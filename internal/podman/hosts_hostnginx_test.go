package podman

import (
	"os/exec"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// With nginx on the host there is no lerd-nginx to point .test at: containers
// reach it through the host gateway, whose 80 and 443 lead to it.
func TestSiteDomainsIP_followsNginxToTheHost(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.SetNginxOnHost(true); err != nil {
		t.Fatal(err)
	}
	prev := execCommand
	t.Cleanup(func() { execCommand = prev })
	execCommand = func(name string, args ...string) *exec.Cmd {
		t.Errorf("inspected a container for nginx on the host: %s %v", name, args)
		return exec.Command("false")
	}
	if got := siteDomainsIP("192.168.127.254"); got != "192.168.127.254" {
		t.Errorf("siteDomainsIP = %q, want the host gateway", got)
	}
}
