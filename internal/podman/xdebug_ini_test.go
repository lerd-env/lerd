package podman

import (
	"strings"
	"testing"
)

// The container image installs xdebug as a package and its own ini loads it, so
// the file only had to carry settings. A native build ships the .so beside the
// binary with nothing loading it, and the debugger listens on the host itself
// rather than across the podman gateway.
func TestXdebugIniContent(t *testing.T) {
	container := xdebugIniContent("debug", "yes", "", 9003)
	if strings.Contains(container, "zend_extension") {
		t.Errorf("container ini must not load the module by path:\n%s", container)
	}
	if !strings.Contains(container, "xdebug.client_host=host.containers.internal") {
		t.Errorf("container ini should reach the host through the gateway:\n%s", container)
	}

	native := xdebugIniContent("debug", "yes", "/m/xdebug.so", 9003)
	if !strings.Contains(native, "zend_extension=/m/xdebug.so") {
		t.Errorf("native ini must load the module by path:\n%s", native)
	}
	if !strings.Contains(native, "xdebug.client_host=127.0.0.1") {
		t.Errorf("native ini should reach the debugger on loopback:\n%s", native)
	}
	if strings.Contains(native, "host.containers.internal") {
		t.Errorf("native ini must not name the podman gateway:\n%s", native)
	}
	for _, want := range []string{"xdebug.mode=debug", "xdebug.start_with_request=yes", "xdebug.client_port=9003"} {
		if !strings.Contains(native, want) {
			t.Errorf("native ini missing %q:\n%s", want, native)
		}
	}
}
