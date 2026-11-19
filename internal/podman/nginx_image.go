package podman

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/registry"
)

const nginxUnit = "lerd-nginx"

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

// readNginxLocal costs two podman inspects per status rebuild. The digests of
// an image ID only change on a pull, so those are kept per ID until one; a
// timed memo here once hid a restart onto another image.
func readNginxLocal() nginxLocal {
	running := ContainerImageID(nginxUnit)
	if running == "" {
		return nginxLocal{}
	}
	return nginxLocal{
		RunningID:      running,
		LocalID:        LocalImageID(ServiceImage(nginxUnit)),
		RunningDigests: imageDigestsFor(running),
	}
}

var (
	nginxLocalFn       = readNginxLocal
	localImageDigestFn = LocalImageDigest
	digestsMu          sync.Mutex
	digestsByID        = map[string][]string{}
	nginxRefreshMu     sync.Mutex
	nginxRefreshOn     bool
	nginxRefreshWG     sync.WaitGroup
)

func imageDigestsFor(id string) []string {
	digestsMu.Lock()
	defer digestsMu.Unlock()
	if d, ok := digestsByID[id]; ok {
		return d
	}
	d := localImageDigestFn(id)
	if len(d) > 0 {
		digestsByID[id] = d
	}
	return d
}

// forgetImageDigests drops the per-ID digests, which a pull of a republished
// tag extends on the image ID it already had.
func forgetImageDigests() {
	digestsMu.Lock()
	digestsByID = map[string][]string{}
	digestsMu.Unlock()
}

// NginxUpdateAvailable reports whether lerd-nginx is behind, answering from
// caches only so a status rebuild never waits on the network. A cold digest
// is fetched in the background and shows up on a later rebuild.
func NginxUpdateAvailable() bool {
	image := ServiceImage(nginxUnit)
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
	latest, err := refreshManifestDigestFn(ServiceImage(nginxUnit))
	if err != nil {
		return nginxBehind(nginxLocalFn(), ""), err
	}
	return nginxBehind(nginxLocalFn(), latest), nil
}

var onNginxDigestRefreshed func()

// SetOnNginxDigestRefreshed registers what runs once a background digest fetch
// finishes, so lerd-ui can rebuild status and announce a republished image.
func SetOnNginxDigestRefreshed(fn func()) {
	nginxRefreshMu.Lock()
	onNginxDigestRefreshed = fn
	nginxRefreshMu.Unlock()
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
	fetch, announce := manifestDigestFn, onNginxDigestRefreshed
	nginxRefreshMu.Unlock()
	go func() {
		defer nginxRefreshWG.Done()
		_, err := fetch(image)
		nginxRefreshMu.Lock()
		nginxRefreshOn = false
		nginxRefreshMu.Unlock()
		if err == nil && announce != nil {
			announce()
		}
	}()
}

// PullNginxImage pulls the web server image and drops the cached registry
// digest, which would otherwise be older than what was just pulled and read
// as an update for the rest of its window.
func PullNginxImage(w io.Writer) error {
	image := ServiceImage(nginxUnit)
	if err := PullImageTo(image, w); err != nil {
		return err
	}
	forgetImageDigests()
	return registry.InvalidateManifestDigest(image)
}

// nginxReadyTimeout matches the restart budget of the global nginx config save.
const nginxReadyTimeout = 20 * time.Second

// UpdateNginx pulls the web server image and, when it moved, restarts
// lerd-nginx onto it. A reload is not enough: it keeps the running binary.
// updated is false when the container already ran the pulled image.
func UpdateNginx(w io.Writer) (updated bool, err error) {
	if err := PullNginxImage(w); err != nil {
		return false, fmt.Errorf("pulling %s: %w", ServiceImage(nginxUnit), err)
	}
	if ContainerImageID(nginxUnit) == LocalImageID(ServiceImage(nginxUnit)) {
		return false, nil
	}
	if err := RestartUnit(nginxUnit); err != nil {
		return false, err
	}
	if err := WaitReady("nginx", nginxReadyTimeout); err != nil {
		return false, err
	}
	return true, nil
}
