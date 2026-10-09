//go:build windows

package nginx

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/platform"
)

// The Podman machine cannot reach a host unix socket, so on Windows the
// dashboard vhost has to proxy to lerd-ui over the gvproxy loopback, as on macOS.
func TestLerdVhostProxiesOverTCPOnWindows(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if !platform.Current.UsesMachineVM {
		t.Skip("not a machine VM host")
	}
	got, err := renderLerdVhost()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "proxy_pass http://host.containers.internal:7073;") {
		t.Errorf("vhost does not proxy to lerd-ui over TCP:\n%s", got)
	}
	if strings.Contains(got, "unix:") {
		t.Errorf("vhost still references a unix socket:\n%s", got)
	}
}
