package watcher

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
)

// autoSnapshotName is the base every scheduled snapshot is named from;
// CreateSnapshot stamps it with the UTC time, so they sort and read as a series.
const autoSnapshotName = "auto"

// Seams for tests: the scheduler is unit-tested without podman or a database.
var (
	autoSnapshotCreate = func(t serviceops.SnapshotTarget, name string, meta serviceops.SnapshotMeta) (*serviceops.Snapshot, error) {
		return serviceops.CreateSnapshot(t, name, meta, nil)
	}
	autoSnapshotPrune     = serviceops.PruneAutoSnapshots
	autoSnapshotSupported = func(service string) bool { return serviceops.SnapshotSupported(service, false) }
	autoSnapshotRunning   = func(service string) bool {
		status, _ := podman.UnitStatus("lerd-" + service)
		return status == "active"
	}
	autoSnapshotStampPathFn = defaultAutoSnapshotStampPath
)

// WatchAutoSnapshot periodically snapshots the database of every site the
// automatic-snapshot policy covers, then prunes what retention has expired. It
// ticks at interval but acts on a target at most once per configured schedule,
// throttled by a persisted stamp so a restarting watcher can't dump more often.
func WatchAutoSnapshot(interval time.Duration) {
	runAutoSnapshots(time.Now())

	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		runAutoSnapshots(time.Now())
	}
}

// runAutoSnapshots performs one throttled pass over the covered databases. Split
// from the ticker so its decision points are unit-testable with an injected clock.
func runAutoSnapshots(now time.Time) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return
	}
	targets := config.AutoSnapshotTargets(cfg)
	if len(targets) == 0 {
		return
	}
	every := cfg.AutoSnapshotEvery()
	policy := serviceops.RetentionPolicy{
		Keep:    cfg.AutoSnapshotKeep(),
		KeepFor: cfg.AutoSnapshotKeepFor(),
		Every:   every,
	}

	stamps := loadAutoSnapshotStamps()
	changed := false
	sites := map[string]bool{}
	took := 0
	for _, t := range targets {
		if now.Sub(stamps[t.Key()]) < every {
			continue
		}
		// A stopped engine is left alone: starting containers behind the user's
		// back to take a backup is a bigger surprise than a missed one, and the
		// next tick picks it up once the engine is back.
		if !autoSnapshotSupported(t.Service) || !autoSnapshotRunning(t.Service) {
			continue
		}
		target := serviceops.SnapshotTarget{Service: t.Service, Family: t.Family, Database: t.Database}
		meta := serviceops.SnapshotMeta{Site: t.Site, GitBranch: autoSnapshotBranch(t.Path), Auto: true}
		snap, err := autoSnapshotCreate(target, autoSnapshotName, meta)
		if err != nil {
			// Don't stamp: the next tick retries instead of waiting a full
			// schedule for a transient engine failure.
			logger.Warn("automatic snapshot failed", "service", t.Service, "database", t.Database, "error", err)
			continue
		}
		stamps[t.Key()] = now
		changed = true
		took++
		sites[t.Site] = true
		logger.Info("automatic snapshot taken", "service", t.Service, "database", t.Database, "name", snap.Name)

		removed, err := autoSnapshotPrune(t.Service, t.Database, policy)
		if err != nil {
			logger.Warn("pruning automatic snapshots failed", "service", t.Service, "database", t.Database, "error", err)
			continue
		}
		if len(removed) > 0 {
			logger.Info("automatic snapshots pruned", "service", t.Service, "database", t.Database, "removed", strings.Join(removed, ","))
		}
	}
	if changed {
		saveAutoSnapshotStamps(stamps)
	}
	// Only a run that took something is worth a notification; a tick the
	// schedule threw away has nothing to report.
	if took > 0 {
		autoSnapshotNotify(took, len(sites))
	}
}

// autoSnapshotNotify hands the finished run to lerd-ui, which owns the single
// notification choke point and the category filter. A daemon that is not up
// simply means no notification, which is not worth failing a snapshot over.
var autoSnapshotNotify = func(databases, sites int) {
	body, err := json.Marshal(map[string]int{"databases": databases, "sites": sites})
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:7073/api/internal/snapshot-run", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

func autoSnapshotBranch(dir string) string {
	out, err := gitpkg.Output(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func defaultAutoSnapshotStampPath() string {
	return filepath.Join(config.DataDir(), "auto-snapshot.stamps")
}

// loadAutoSnapshotStamps reads when each target was last snapshotted. A missing
// or unreadable file leaves every target due, which is the safe direction.
func loadAutoSnapshotStamps() map[string]time.Time {
	stamps := map[string]time.Time{}
	data, err := os.ReadFile(autoSnapshotStampPathFn())
	if err != nil {
		return stamps
	}
	var raw map[string]int64
	if err := json.Unmarshal(data, &raw); err != nil {
		return stamps
	}
	for key, secs := range raw {
		stamps[key] = time.Unix(secs, 0)
	}
	return stamps
}

func saveAutoSnapshotStamps(stamps map[string]time.Time) {
	raw := make(map[string]int64, len(stamps))
	for key, at := range stamps {
		raw[key] = at.Unix()
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return
	}
	_ = os.WriteFile(autoSnapshotStampPathFn(), data, 0600)
}
