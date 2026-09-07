package serviceops

import (
	"fmt"
	"os"
	"sort"
	"time"
)

// RetentionPolicy is the effective automatic-snapshot retention: how many
// snapshots survive per database, how old one may get, and how often the
// schedule runs. Every surface explains expiry through this one policy, so what
// a snapshot list promises is what the prune actually does.
type RetentionPolicy struct {
	Keep    int           // automatic snapshots kept per database; 0 means no count limit
	KeepFor time.Duration // maximum age; 0 means age never expires one
	Every   time.Duration // schedule interval, which dates the count-based estimate
}

// SnapshotExpiry says when one snapshot goes away. A manual snapshot is never
// pruned and reports the zero value.
type SnapshotExpiry struct {
	Kept      bool      // pinned by the user, exempt from retention
	At        time.Time // when it expires; zero when nothing expires it
	RunsLeft  int       // scheduled runs it still survives, under the count rule
	Estimated bool      // At was derived from the count rule, so it moves with the schedule
}

// SnapshotExpiries answers, for each snapshot in snaps, when retention will drop
// it. The result is index-aligned with the input. Snapshots are ranked inside
// their own (service, database, scope) window, so a mixed list from several
// databases is answered correctly in one pass. Kept snapshots take no slot in
// the window: pinning one preserves it without shrinking the rolling set.
func SnapshotExpiries(p RetentionPolicy, snaps []Snapshot) []SnapshotExpiry {
	out := make([]SnapshotExpiry, len(snaps))

	for _, idx := range prunableGroups(snaps) {
		base := snaps[idx[0]].Created
		for rank, i := range idx {
			out[i] = expiryAt(p, snaps[i].Created, base, rank)
		}
	}
	for i, s := range snaps {
		if s.Auto && s.Kept {
			out[i] = SnapshotExpiry{Kept: true}
		}
	}
	return out
}

// expiryAt resolves one snapshot's expiry from whichever rule fires first: the
// count rule, dated off base (the newest snapshot in its window, which stands in
// for the last scheduled run), or the exact age cutoff.
func expiryAt(p RetentionPolicy, created, base time.Time, rank int) SnapshotExpiry {
	var e SnapshotExpiry
	if p.Keep > 0 {
		e.RunsLeft = max(p.Keep-rank, 0)
		if p.Every > 0 {
			e.At = base.Add(time.Duration(e.RunsLeft) * p.Every)
			e.Estimated = true
		}
	}
	if p.KeepFor > 0 {
		age := created.Add(p.KeepFor)
		if e.At.IsZero() || age.Before(e.At) {
			e.At, e.Estimated = age, false
		}
	}
	return e
}

// prunableGroups indexes the snapshots retention actually acts on, grouped by
// the window they compete in and ordered newest first inside each group.
func prunableGroups(snaps []Snapshot) map[string][]int {
	groups := map[string][]int{}
	for i, s := range snaps {
		if !s.Auto || s.Kept {
			continue
		}
		key := fmt.Sprintf("%s\x00%s\x00%t", s.Service, s.Database, s.AllDatabases)
		groups[key] = append(groups[key], i)
	}
	for _, idx := range groups {
		sort.SliceStable(idx, func(a, b int) bool {
			return snaps[idx[a]].Created.After(snaps[idx[b]].Created)
		})
	}
	return groups
}

// PruneAutoSnapshots deletes the automatic snapshots of one database that the
// policy has expired, and returns the names it removed. Manual snapshots and
// kept ones are never touched.
func PruneAutoSnapshots(service, database string, p RetentionPolicy) ([]string, error) {
	if p.Keep <= 0 && p.KeepFor <= 0 {
		return nil, nil
	}
	snaps, err := ListSnapshots(service, database, false)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var removed []string
	for _, idx := range prunableGroups(snaps) {
		for rank, i := range idx {
			s := snaps[i]
			overCount := p.Keep > 0 && rank >= p.Keep
			overAge := p.KeepFor > 0 && now.Sub(s.Created) > p.KeepFor
			if !overCount && !overAge {
				continue
			}
			if err := DeleteSnapshot(s.Service, s.Database, s.Name, s.AllDatabases); err != nil {
				return removed, fmt.Errorf("pruning snapshot %q: %w", s.Name, err)
			}
			removed = append(removed, s.Name)
		}
	}
	sort.Strings(removed)
	return removed, nil
}

// SetSnapshotKept pins an automatic snapshot so retention never expires it, or
// releases it back under retention. A manual snapshot is already permanent, so
// asking to keep one is an error rather than a silent no-op.
func SetSnapshotKept(service, database, name string, allDatabases, kept bool) error {
	if !allDatabases {
		if err := ValidateDatabaseName(database); err != nil {
			return err
		}
	}
	clean, err := sanitizeSnapshotName(name)
	if err != nil {
		return err
	}
	dir := snapshotDir(service, database, clean, allDatabases)
	snap, err := readSnapshotMeta(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("snapshot %q not found", name)
		}
		return fmt.Errorf("reading snapshot %q: %w", name, err)
	}
	if !snap.Auto {
		return fmt.Errorf("snapshot %q was taken by hand and is already permanent", name)
	}
	if snap.Kept == kept {
		return nil
	}
	snap.Kept = kept
	return writeSnapshotMeta(dir, snap)
}

// Label renders an expiry for a snapshot list: "kept" for a pinned snapshot,
// "~3d" for the count rule's moving estimate, "3d" for an exact age cutoff, and
// empty when nothing expires the snapshot at all.
func (e SnapshotExpiry) Label(now time.Time) string {
	switch {
	case e.Kept:
		return "kept"
	case e.At.IsZero():
		return ""
	case !e.At.After(now):
		return "due now"
	case e.Estimated:
		return "~" + compactDuration(e.At.Sub(now))
	}
	return compactDuration(e.At.Sub(now))
}

// compactDuration renders a duration as the largest single unit (41m, 2h, 3d).
func compactDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}
