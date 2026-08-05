package podman

import (
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
