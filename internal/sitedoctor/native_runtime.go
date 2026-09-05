package sitedoctor

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
)

// nativeListenerCheck reports whether the host PHP-FPM a native site is served
// by is answering. When it is not, nginx returns 502 with nothing in the
// project's own logs to explain it, so this is the one place that says why.
func nativeListenerCheck(version string, port int, listening func(int) bool) Check {
	c := Check{Name: "native runtime", Status: StatusOK,
		Detail: fmt.Sprintf("php %s serving on the host (port %d)", version, port)}
	if !listening(port) {
		c.Status = StatusFail
		c.Detail = fmt.Sprintf("nothing is listening on port %d, so every request 502s; start it with 'lerd runtime native'", port)
	}
	return c
}

// nativeExtensionCheck compares what a project requires against what the native
// build carries. The set is fixed at build time, so an extension that is merely
// missing here would surface as a fatal deep inside a request with nothing
// pointing at the runtime.
func nativeExtensionCheck(required, available []string) Check {
	have := make(map[string]bool, len(available))
	for _, e := range available {
		have[podman.CanonicalExtension(e)] = true
	}
	var missing []string
	for _, e := range required {
		if !have[podman.CanonicalExtension(e)] {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return Check{Name: "native extensions", Status: StatusOK,
			Detail: "the native build carries everything this project requires"}
	}
	sort.Strings(missing)
	return Check{Name: "native extensions", Status: StatusWarn,
		Detail: fmt.Sprintf("the native build has no %s; switch this site back with 'lerd runtime fpm' if it needs them",
			strings.Join(missing, ", "))}
}

// portListening dials loopback to see whether a listener is up.
func portListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// checkNativeRuntime runs the native checks for a site, and nothing at all for
// a site on the container runtime.
func checkNativeRuntime(path string) ([]Check, bool) {
	site, err := config.FindSiteByPath(path)
	if err != nil || site == nil || !site.IsNative() {
		return nil, false
	}
	port, err := nativephp.PortFor(site.PHPVersion)
	if err != nil {
		return nil, false
	}
	checks := []Check{nativeListenerCheck(site.PHPVersion, port, portListening)}
	// Only worth reporting when both sides are known: a build we cannot read
	// tells us nothing about what the project is missing.
	if available, err := nativephp.Extensions(site.PHPVersion); err == nil {
		if required := phpDet.DetectExtensions(path); len(required) > 0 {
			checks = append(checks, nativeExtensionCheck(required, available))
		}
	}
	checks = append(checks, nativeQueryCaptureCheck(nativeHasQueryCapture(site.PHPVersion)))
	return checks, true
}

// nativeQueryCaptureCheck reports whether the Debug window's query lens can
// capture anything. That capture is engine-level, from the lerd_devtools
// extension the PHP image compiles in; the native build does not carry it yet,
// and an empty lens with no explanation reads as a broken feature rather than
// a runtime that does not provide it.
func nativeQueryCaptureCheck(present bool) Check {
	if present {
		return Check{Name: "query capture", Status: StatusOK,
			Detail: "the Debug window can capture queries on this runtime"}
	}
	return Check{Name: "query capture", Status: StatusWarn,
		Detail: "the native runtime cannot capture queries for the Debug window; dump() and dd() still work, and 'lerd php:runtime container' restores the query lens"}
}

// nativeHasQueryCapture reports whether the native runtime can capture queries,
// which it can once the build ships the collector extension beside the binary.
func nativeHasQueryCapture(version string) bool {
	return nativephp.DevtoolsExtensionPath(version) != ""
}
