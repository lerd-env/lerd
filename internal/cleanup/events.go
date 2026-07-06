package cleanup

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// autoEnabled reports whether automatic cleanup is on. Seam for tests.
var autoEnabled = defaultAutoEnabled

func defaultAutoEnabled() bool {
	cfg, err := config.LoadGlobal()
	return err == nil && cfg.AutoCleanupEnabled()
}

// SweepSafe runs the safe-tier cleanup immediately when auto_cleanup is on. It
// is the event hook for the moment a PHP image rebuild orphans the old image,
// reaping it at once instead of waiting for the daily watcher.
func SweepSafe() (images int, bytes int64, err error) { return sweep(false) }

// SweepDeep runs the deep tier: the safe-tier orphans plus service catalog
// images no service references any more. This is the daily watcher's sweep, so
// old service versions left by an upgrade get reclaimed unattended. Protected
// images (the current one and the one-back rollback) and user-added tags are
// always kept, and the deep tier degrades to safe when the preset store can't
// be read, so the unattended path stays safe.
func SweepDeep() (images int, bytes int64, err error) { return sweep(true) }

// sweep runs a cleanup tier when auto_cleanup is on. Returns the number of
// images reaped and bytes freed; err is non-nil only when the image scan itself
// failed, so the watcher can tell a transient failure (retry) from "nothing to
// do" (throttle). All-zero with nil err when disabled or clean.
func sweep(deep bool) (images int, bytes int64, err error) {
	if !autoEnabled() {
		return 0, 0, nil
	}
	plan, err := Inspect(deep)
	if err != nil {
		return 0, 0, err
	}
	// Report what Apply actually removed, not the planned count: a target that
	// became referenced again since Inspect is skipped, so the plan can overstate
	// the reclaim.
	removed, bytesFreed := Apply(plan)
	return removed, bytesFreed, nil
}

// SweepRefs reaps the exact image references lerd is dropping (the superseded
// version after a service update, the removed service's images after a remove).
// Each ref is one lerd itself recorded, so it is provably lerd's; a ref another
// service still references (in the protected set) is skipped. Reaping precise
// refs rather than a whole repo means a user's own same-repo image is never
// touched. Gated by auto_cleanup.
func SweepRefs(refs ...string) {
	if !autoEnabled() {
		return
	}
	// De-dup the non-empty refs into canonical candidates, preserving order so a
	// repeated ref is reaped once. Apply the host image rewrite first
	// (postgis/postgis -> imresamu/postgis on Apple Silicon) so the ref matches the
	// name actually stored, otherwise the superseded image is never reclaimed.
	candidates := map[string]bool{}
	var order []string
	for _, ref := range refs {
		if ref == "" {
			continue
		}
		ref = podman.PlatformImage(ref)
		c := canonRef(ref)
		if candidates[c] {
			continue
		}
		candidates[c] = true
		order = append(order, ref)
	}
	if len(candidates) == 0 {
		return
	}
	protected := referencedImages(candidates)
	for _, ref := range order {
		if protected[canonRef(ref)] {
			continue
		}
		_ = removeImage(ref)
	}
}
