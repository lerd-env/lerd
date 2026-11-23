package podman

import (
	"strings"
	"testing"
)

// Every container mounts this file, the FPM, FrankenPHP and custom-FPM ones and
// worker units, under the native runtime too. The image's own ini loads xdebug,
// and the IDE is reached across the podman gateway.
func TestXdebugIniContent(t *testing.T) {
	got := xdebugIniContent("debug", "yes", 9003)
	if strings.Contains(got, "zend_extension") {
		t.Errorf("a container cannot load a host module by path:\n%s", got)
	}
	if !strings.Contains(got, "xdebug.client_host=host.containers.internal") {
		t.Errorf("a container should reach the IDE through the gateway:\n%s", got)
	}
	for _, want := range []string{"xdebug.mode=debug", "xdebug.start_with_request=yes", "xdebug.client_port=9003"} {
		if !strings.Contains(got, want) {
			t.Errorf("ini missing %q:\n%s", want, got)
		}
	}
}
