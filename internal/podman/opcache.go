package podman

import "runtime"

// goosForFPM is the platform the FPM unit is rendered for, a seam for tests.
var goosForFPM = runtime.GOOS

// fpmArgs is appended to the FPM container's command. On Windows the project
// is on a 9p share where each of the thousands of stats OPcache revalidation
// makes per request crosses to the host, so revalidation is off there and lerd
// drops OPcache entries itself when files change (InvalidateOPcache).
func fpmArgs(goos string) string {
	if goos == "windows" {
		return " -d opcache.validate_timestamps=0"
	}
	return ""
}

// FPMSkipsRevalidation reports whether this host's FPM containers keep compiled
// files until lerd flushes them.
func FPMSkipsRevalidation() bool { return fpmArgs(goosForFPM) != "" }

// OPcache invalidation modes, see opcache-invalidate.php. OPcacheApp drops every
// cached script outside vendor/, which covers edited code and the templates and
// caches a framework compiles from it, while the dependencies stay compiled.
// OPcacheReset drops everything, for when the dependencies changed.
const (
	OPcacheApp   = "app"
	OPcacheReset = "reset"
)

// InvalidateOPcache drops OPcache entries in the php-fpm pool of container.
func InvalidateOPcache(container, mode string) error {
	return Cmd("exec", container, "php", "/usr/local/etc/lerd/opcache-invalidate.php", mode).Run()
}
