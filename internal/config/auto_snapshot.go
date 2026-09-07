package config

import (
	"fmt"
	"strings"
	"time"
)

// Automatic snapshots are ordinary snapshots taken on a schedule. A site's
// AutoSnapshot field is a tri-state override of the global policy, so a machine
// can back nothing up except two named sites, or back everything up except one
// noisy site, without two separate settings.
const (
	AutoSnapshotDefault = ""    // follow the global policy
	AutoSnapshotOn      = "on"  // always snapshot this site
	AutoSnapshotOff     = "off" // never snapshot this site
)

// The schedule's selection mode: whether a site is covered until it opts out,
// or covered only once it opts in. The per-site override wins either way.
const (
	AutoSnapshotOptIn  = "opt_in"  // no site is covered unless it says yes (default)
	AutoSnapshotOptOut = "opt_out" // every site is covered unless it says no
)

// DefaultAutoSnapshotEvery is the gap between scheduled snapshots when no
// interval is configured.
const DefaultAutoSnapshotEvery = 24 * time.Hour

// DefaultAutoSnapshotKeep is how many automatic snapshots survive per database
// when no retention count is configured.
const DefaultAutoSnapshotKeep = 7

// AutoSnapshotEnabled reports whether the scheduled-snapshot policy is on.
// Nil-safe and on by default, matching defaultConfig; an unconfigured install
// still dumps nothing, because the default selection mode covers no database
// until one is opted in.
func (c *GlobalConfig) AutoSnapshotEnabled() bool {
	return c == nil || c.AutoSnapshot.Enabled
}

// AutoSnapshotEvery returns the effective schedule interval, falling back to
// DefaultAutoSnapshotEvery when unset, unparseable, or non-positive.
func (c *GlobalConfig) AutoSnapshotEvery() time.Duration {
	if c != nil && c.AutoSnapshot.Every != "" {
		if d, err := time.ParseDuration(c.AutoSnapshot.Every); err == nil && d > 0 {
			return d
		}
	}
	return DefaultAutoSnapshotEvery
}

// AutoSnapshotKeep returns how many automatic snapshots survive per database.
// Zero means no count limit, which a negative configured value asks for.
func (c *GlobalConfig) AutoSnapshotKeep() int {
	if c == nil || c.AutoSnapshot.Keep == 0 {
		return DefaultAutoSnapshotKeep
	}
	if c.AutoSnapshot.Keep < 0 {
		return 0
	}
	return c.AutoSnapshot.Keep
}

// AutoSnapshotKeepFor returns the maximum age of an automatic snapshot, or zero
// when age alone never expires one.
func (c *GlobalConfig) AutoSnapshotKeepFor() time.Duration {
	if c != nil && c.AutoSnapshot.KeepFor != "" {
		if d, err := time.ParseDuration(c.AutoSnapshot.KeepFor); err == nil && d > 0 {
			return d
		}
	}
	return 0
}

// AutoSnapshotSelection returns the effective selection mode, defaulting to
// opt-in: the schedule ships on, and covering every database found on the
// machine without being asked is not a default worth shipping.
func (c *GlobalConfig) AutoSnapshotSelection() string {
	if c != nil && c.AutoSnapshot.Selection == AutoSnapshotOptOut {
		return AutoSnapshotOptOut
	}
	return AutoSnapshotOptIn
}

// NormalizeAutoSnapshotSelection validates a user-supplied selection mode,
// accepting the hyphenated spelling the CLI and the API use.
func NormalizeAutoSnapshotSelection(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "opt-in", AutoSnapshotOptIn:
		return AutoSnapshotOptIn, nil
	case "opt-out", AutoSnapshotOptOut:
		return AutoSnapshotOptOut, nil
	}
	return "", fmt.Errorf("unknown selection mode %q — use opt-in or opt-out", mode)
}

// AutoSnapshotCovers reports whether the site's database is on the schedule,
// resolving its tri-state override against the global policy.
func (c *GlobalConfig) AutoSnapshotCovers(s *Site) bool {
	if s == nil {
		return false
	}
	return c.AutoSnapshotModeCovers(s.AutoSnapshot)
}

// AutoSnapshotModeCovers resolves one site override against the global policy,
// for callers that hold the mode without the site record.
func (c *GlobalConfig) AutoSnapshotModeCovers(mode string) bool {
	// Off is off: the schedule's own switch gates every site, including one that
	// opted in. Covering a database while the user has turned the schedule off
	// would make the switch a lie, and the selection mode is what expresses
	// "only these sites".
	if !c.AutoSnapshotEnabled() {
		return false
	}
	switch mode {
	case AutoSnapshotOn:
		return true
	case AutoSnapshotOff:
		return false
	}
	// An unconfigured site follows the selection mode: covered by default under
	// opt-out, left alone under opt-in until it says otherwise.
	return c.AutoSnapshotSelection() == AutoSnapshotOptOut
}

// NormalizeAutoSnapshotMode validates a user-supplied site override, accepting
// "default" as the spelling of the empty follow-the-global value.
func NormalizeAutoSnapshotMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "default":
		return AutoSnapshotDefault, nil
	case AutoSnapshotOn:
		return AutoSnapshotOn, nil
	case AutoSnapshotOff:
		return AutoSnapshotOff, nil
	}
	return "", fmt.Errorf("unknown automatic-snapshot mode %q — use on, off, or default", mode)
}

// SetSiteAutoSnapshot atomically updates just a site's automatic-snapshot
// override, the same single-field rewrite under the write lock the other
// registry mutators do.
func SetSiteAutoSnapshot(name, mode string) error {
	clean, err := NormalizeAutoSnapshotMode(mode)
	if err != nil {
		return err
	}
	siteWriteMu.Lock()
	defer siteWriteMu.Unlock()
	reg, err := LoadSites()
	if err != nil {
		return err
	}
	for i := range reg.Sites {
		if reg.Sites[i].Name == name {
			reg.Sites[i].AutoSnapshot = clean
			return SaveSites(reg)
		}
	}
	return fmt.Errorf("site %q not found", name)
}

// AutoSnapshotTarget is one database on the schedule, with the site context a
// stored snapshot records so it still says where it came from.
type AutoSnapshotTarget struct {
	Service  string
	Family   string
	Database string
	Site     string
	Domain   string
	Path     string
	// Mode is the site's own tri-state override, carried so a surface can offer
	// the opt-in/out without loading the registry again.
	Mode string
}

// Key identifies the database a target snapshots, which is what the schedule is
// throttled on: two sites sharing a database share one entry.
func (t AutoSnapshotTarget) Key() string { return t.Service + "\x00" + t.Database }

// AutoSnapshotSiteTargets returns every database a linked site points at, one
// entry per site so each carries its own override, whether or not the policy
// currently covers it. Ignored sites are not lerd's to back up.
func AutoSnapshotSiteTargets() []AutoSnapshotTarget {
	reg, err := LoadSites()
	if err != nil {
		return nil
	}
	var out []AutoSnapshotTarget
	for i := range reg.Sites {
		site := &reg.Sites[i]
		if site.Ignored {
			continue
		}
		for _, t := range DBTargetsFor(site.Path) {
			out = append(out, AutoSnapshotTarget{
				Service:  t.Service,
				Family:   autoSnapshotFamily(t.Service),
				Database: t.Database,
				Site:     site.Name,
				Domain:   site.PrimaryDomain(),
				Path:     site.Path,
				Mode:     site.AutoSnapshot,
			})
		}
	}
	return out
}

// AutoSnapshotTargets returns the databases the schedule acts on, deduped: a
// group sharing one database, or two sites on the same one, is a single target
// rather than a repeated dump of the same data.
func AutoSnapshotTargets(cfg *GlobalConfig) []AutoSnapshotTarget {
	seen := map[string]bool{}
	var out []AutoSnapshotTarget
	for _, t := range AutoSnapshotSiteTargets() {
		if !cfg.AutoSnapshotModeCovers(t.Mode) || seen[t.Key()] {
			continue
		}
		seen[t.Key()] = true
		out = append(out, t)
	}
	return out
}

// autoSnapshotFamily is the engine family a snapshot records, falling back to
// the service name for an engine that declares no family of its own.
func autoSnapshotFamily(service string) string {
	if family := FamilyOfName(service); family != "" {
		return family
	}
	return service
}
