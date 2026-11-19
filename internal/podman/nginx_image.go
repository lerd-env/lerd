package podman

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/registry"
)

// nginxLocal is what the machine knows about the web server image: the image
// lerd-nginx runs, the image its tag now points at locally, and the registry
// digests recorded for the running one.
type nginxLocal struct {
	RunningID      string
	LocalID        string
	RunningDigests []string
}

// nginxBehind reports whether lerd-nginx runs an older image than it could.
// nginx:alpine floats, so a fix is the same tag with a new digest. latest is ""
// when the registry has not answered, which must never read as an update.
func nginxBehind(local nginxLocal, latest string) bool {
	if local.RunningID == "" {
		return false
	}
	if local.LocalID != "" && local.LocalID != local.RunningID {
		return true
	}
	if latest == "" {
		return false
	}
	for _, d := range local.RunningDigests {
		if strings.EqualFold(strings.TrimSpace(d), strings.TrimSpace(latest)) {
			return false
		}
	}
	return true
}

// readNginxLocal reads fresh on every status rebuild, three podman inspects. A
// pull in another process can add a digest to the same image ID, so any cache
// held here would keep a finished update flagged.
func readNginxLocal() nginxLocal {
	running := containerImageIDFn(WebServerUnit)
	if running == "" {
		return nginxLocal{}
	}
	return nginxLocal{
		RunningID:      running,
		LocalID:        localImageIDFn(ServiceImage(WebServerUnit)),
		RunningDigests: localImageDigestFn(running),
	}
}

var (
	nginxLocalFn       = readNginxLocal
	localImageDigestFn = LocalImageDigest
	containerImageIDFn = ContainerImageID
	localImageIDFn     = LocalImageID
	nginxRefreshMu     sync.Mutex
	nginxRefreshOn     bool
	nginxRefreshWG     sync.WaitGroup
)

// NginxUpdateAvailable reports whether lerd-nginx is behind, answering from
// caches only so a status rebuild never waits on the network. A cold digest
// is fetched in the background and shows up on a later rebuild.
func NginxUpdateAvailable() bool {
	image := ServiceImage(WebServerUnit)
	latest, ok := cachedManifestDigestFn(image)
	if !ok {
		scheduleNginxDigestRefresh(image)
	}
	return nginxBehind(nginxLocalFn(), latest)
}

// CheckNginxUpdate re-reads the tag's digest from the registry, bypassing the
// cache. Blocking, for the manual check. An unreachable registry is returned
// as an error rather than read as "up to date".
func CheckNginxUpdate() (bool, error) {
	latest, err := refreshManifestDigestFn(ServiceImage(WebServerUnit))
	if err != nil {
		return nginxBehind(nginxLocalFn(), ""), err
	}
	return nginxBehind(nginxLocalFn(), latest), nil
}

// scheduleNginxDigestRefresh reads through the digest cache, failures included,
// so an offline machine is asked once per cache window rather than every rebuild.
func scheduleNginxDigestRefresh(image string) {
	nginxRefreshMu.Lock()
	if nginxRefreshOn {
		nginxRefreshMu.Unlock()
		return
	}
	nginxRefreshOn = true
	nginxRefreshWG.Add(1)
	fetch := manifestDigestFn
	nginxRefreshMu.Unlock()
	go func() {
		defer nginxRefreshWG.Done()
		_, err := fetch(image)
		nginxRefreshMu.Lock()
		nginxRefreshOn = false
		nginxRefreshMu.Unlock()
		if err == nil {
			announceBackgroundRead(WebServerUnit)
		}
	}()
}

// PullNginxImage pulls the web server image and drops the cached registry
// digest, which would otherwise be older than what was just pulled and read
// as an update for the rest of its window.
func PullNginxImage(w io.Writer) error {
	image := ServiceImage(WebServerUnit)
	if err := PullImageTo(image, w); err != nil {
		return err
	}
	err := registry.InvalidateManifestDigest(image)
	// A pull can move the update answer without restarting nginx, so lerd-ui is
	// told the same way a restart would tell it.
	notifyUnitChange(WebServerUnit)
	return err
}

// nginxReadyTimeout matches the restart budget of the global nginx config save.
const nginxReadyTimeout = 20 * time.Second

// UpdateNginx pulls the web server image and, when it moved, restarts
// lerd-nginx onto it. A reload is not enough: it keeps the running binary.
// updated is false when the container already ran the pulled image.
func UpdateNginx(w io.Writer) (updated bool, err error) {
	if err := PullNginxImage(w); err != nil {
		return false, fmt.Errorf("pulling %s: %w", ServiceImage(WebServerUnit), err)
	}
	if ContainerImageID(WebServerUnit) == LocalImageID(ServiceImage(WebServerUnit)) {
		return false, nil
	}
	if err := RestartUnit(WebServerUnit); err != nil {
		return false, err
	}
	if err := WaitReady("nginx", nginxReadyTimeout); err != nil {
		return false, err
	}
	return true, nil
}
