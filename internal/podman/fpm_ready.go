package podman

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// fpmReadyProbe asks PHP inside the container whether php-fpm is accepting
// connections on the port nginx proxies to. PHP is the one interpreter an FPM
// image is guaranteed to carry, so this needs no extra tooling in the image.
var fpmReadyProbe = []string{"php", "-r", `exit(@fsockopen("127.0.0.1", 9000) ? 0 : 1);`}

// FPMUnitName is the systemd unit and container name for a PHP version.
func FPMUnitName(version string) string {
	return "lerd-php" + strings.ReplaceAll(version, ".", "") + "-fpm"
}

// EnsureFPMReady starts a version's FPM unit when it is not running and waits
// until php-fpm accepts connections. Switching a site's version repoints its
// vhost at that backend immediately, so without this the first request after
// the switch reaches a container that is still booting and gets a 502. The wait
// is bounded: a version whose image is missing or broken must not hold the
// command open, the site simply reports the failure the next request makes.
func EnsureFPMReady(version string, timeout time.Duration) error {
	unit := FPMUnitName(version)
	if running, _ := ContainerRunning(unit); !running {
		if err := StartUnit(unit); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		args := append([]string{"exec", unit}, fpmReadyProbe...)
		if execCommand(PodmanBin(), args...).Run() == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil
}

// isFPMUnit reports whether a unit name is one of the per-version PHP-FPM
// containers, as opposed to nginx, a service or a worker.
func isFPMUnit(unit string) bool {
	return strings.HasPrefix(unit, "lerd-php") && strings.HasSuffix(unit, "-fpm")
}

// errFPMNotAccepting is the sentinel behind a pool that never came back, so a
// caller can tell it apart from a transport failure.
var errFPMNotAccepting = errors.New("php-fpm did not start accepting")

// fpmRestartReadyTimeout bounds the wait after a restart. A pool that is coming
// back at all accepts within a second; past this it is a real failure and the
// caller should hear about it rather than wait.
const fpmRestartReadyTimeout = 30 * time.Second

// dropNginxUpstreamCache makes nginx forget the address it resolved for the FPM
// containers. A restart moves the container to a new address on the lerd
// network, and nginx caches the old one for its resolver's TTL, so a request in
// that window connects to an address nothing answers on and waits out
// fastcgi_connect_timeout before returning 504. A reload rereads the config and
// starts the resolver cache empty, which is the whole point here; it is a signal
// to the running nginx, keeps existing connections, and costs a few
// milliseconds. Best effort: nginx not running is not this function's problem,
// and the caller's restart still succeeded.
var dropNginxUpstreamCache = func() {
	_, _ = Run("exec", "lerd-nginx", "nginx", "-s", "reload")
}

// waitFPMAccepting polls the same in-container probe EnsureFPMReady uses, but
// reports the timeout instead of swallowing it: a restart that is still not
// accepting has failed, and the caller is about to tell the user it succeeded.
func waitFPMAccepting(unit string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		args := append([]string{"exec", unit}, fpmReadyProbe...)
		if execCommand(PodmanBin(), args...).Run() == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: %w within %s", unit, errFPMNotAccepting, timeout)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
