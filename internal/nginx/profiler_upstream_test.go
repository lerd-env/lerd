package nginx

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The profiler vhost routes to whatever runs PHP. Pointed at the FPM container
// under the native runtime, where nothing is listening on it, so SPX's own
// dashboard answered 502.
func TestProfilerVhostFollowsTheRuntime(t *testing.T) {
	native := profilerVhost(config.PHPRuntimeNative, "8.4", "/state", "on")
	// The host and port are separate directives, as in a site vhost.
	if !strings.Contains(native, `"host.containers.internal"`) || !strings.Contains(native, "$fpm:9484") {
		t.Errorf("native vhost should reach the host listener:\n%s", native)
	}
	if strings.Contains(native, "lerd-php84-fpm") {
		t.Errorf("native vhost must not name the FPM container:\n%s", native)
	}
	// The bridge is the script SPX intercepts in front of, and its container
	// path does not exist on a host running PHP natively.
	if strings.Contains(native, "/usr/local/etc/lerd/dump-bridge.php") {
		t.Errorf("native vhost must not use the image's asset path:\n%s", native)
	}

	// The bridge is auto-prepended and is also the script, so the prepend has
	// to be switched off or the second load fatals on redeclaring it.
	if !strings.Contains(native, `PHP_VALUE "auto_prepend_file="`) {
		t.Errorf("the vhost must switch the prepend off:\n%s", native)
	}

	container := profilerVhost(config.PHPRuntimeContainer, "8.4", "/state", "on")
	if !strings.Contains(container, "lerd-php84-fpm") || !strings.Contains(container, "$fpm:9000") {
		t.Errorf("container vhost should reach the FPM container:\n%s", container)
	}
	if !strings.Contains(container, "/usr/local/etc/lerd/dump-bridge.php") {
		t.Errorf("container vhost keeps the image's asset path:\n%s", container)
	}
}

// A version with no listener port of its own must not render a vhost pointing
// at port 0, which would fail nginx -t and take every site down with it.
func TestProfilerVhostFallsBackForABadVersion(t *testing.T) {
	out := profilerVhost(config.PHPRuntimeNative, "nonsense", "/state", "off")
	if strings.Contains(out, ":0") {
		t.Errorf("a bad version must not render port 0:\n%s", out)
	}
}
