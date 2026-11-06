// Package dbview assembles the Databases surface: which installed services are
// database engines, the databases inside each one with their sizes, the site
// that owns each database, and the snapshots it holds. The web UI and the TUI
// both render from here so the two can never disagree about what a database
// belongs to.
package dbview

import (
	"sort"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/siteinfo"
)

// TestingSuffix names the paired testing database lerd creates alongside every
// project database.
const TestingSuffix = "_testing"

// Owner is the site a database belongs to: the parent site's domain, plus the
// branch when the database is a worktree's isolated one. The branch is what
// turns "astrolov_staging" into staging.astrolov.test in the UI.
type Owner struct {
	Domain string
	Branch string
}

// Entry is a single database with its size, its owning site and the snapshots
// taken of it.
type Entry struct {
	Name      string
	SizeBytes int64
	Owner     Owner
	Snapshots []serviceops.Snapshot
}

// Engine is one database engine with the databases it holds. Error carries an
// introspection failure so the surface can say why an engine came back empty.
type Engine struct {
	Service          string
	Family           string
	Running          bool
	SupportsSnapshot bool
	Databases        []Entry
	Error            string
}

// SiteIndexes maps each engine to the databases owned in it, keyed by database
// name, resolved through each site's framework declaration and from the
// isolated databases worktrees have registered. A "<db>_testing" database maps
// to the same owner as "<db>", so both link to the same place. When a group
// shares one database across a main site and its secondaries, the database
// belongs to the group main, so a secondary that merely shares it never wins
// over the main.
//
// Every engine is answered from one pass over the sites: resolving a site's
// targets detects its framework, which is far too much work to repeat per
// engine on every poll of the Databases tab.
func SiteIndexes() map[string]map[string]Owner {
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	byService := map[string]map[string]Owner{}
	idxFor := func(service string) map[string]Owner {
		if byService[service] == nil {
			byService[service] = map[string]Owner{}
		}
		return byService[service]
	}
	// authoritative[db] is true once db is claimed by a site that owns it rather
	// than a secondary sharing the group's database.
	authoritative := map[string]map[string]bool{}
	claim := func(service, db string, owner Owner, owns bool) {
		idx := idxFor(service)
		if authoritative[service] == nil {
			authoritative[service] = map[string]bool{}
		}
		if _, seen := idx[db]; !seen || (!authoritative[service][db] && owns) {
			idx[db] = owner
			authoritative[service][db] = owns
		}
	}
	domains := map[string]string{}
	for _, s := range reg.Sites {
		if s.Ignored {
			continue
		}
		domains[s.Name] = s.PrimaryDomain()
		owns := !(s.IsGroupSecondary() && s.GroupSharedDB)
		owner := Owner{Domain: s.PrimaryDomain()}
		for _, t := range config.DBTargetsFor(s.Path) {
			if t.Database == "" {
				continue
			}
			claim(t.Service, t.Database, owner, owns)
			claim(t.Service, t.Database+TestingSuffix, owner, owns)
		}
	}
	entries, err := config.LoadWorktreeDBRegistry()
	if err != nil {
		return byService
	}
	for _, e := range entries {
		domain := domains[e.Site]
		if e.DBName == "" || domain == "" {
			continue
		}
		owner := Owner{Domain: domain, Branch: e.Branch}
		claim(e.Service, e.DBName, owner, true)
		claim(e.Service, e.DBName+TestingSuffix, owner, true)
	}
	return byService
}

// IsEngine reports whether a service belongs on the Databases surface: a family
// lerd wires as a project database, or any engine whose preset declares
// databases it can enumerate. The second half is what lets the store publish an
// engine outside the wired families (an analytics column store, say) and have
// it appear with the operations it declares.
func IsEngine(name string) bool {
	return config.IsDBServiceName(name) || serviceops.DeclaresDatabases(name)
}

// InstalledEngines returns the installed database-engine service names, both
// default-stack (mysql, postgres) and add-on (mariadb, mongo, postgres-pgvector).
// sqlite is a file-based engine with no container, so it is excluded.
func InstalledEngines() []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if name == "sqlite" || seen[name] || !IsEngine(name) {
			return
		}
		if !serviceops.ServiceInstalled(name) {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, name := range siteinfo.KnownServices() {
		add(name)
	}
	if customs, err := config.ListCustomServices(); err == nil {
		for _, svc := range customs {
			add(svc.Name)
		}
	}
	sort.Strings(names)
	return names
}

// Load builds one engine's view, introspecting its databases and snapshots only
// when the container is running. siteIndex is that engine's slice of
// SiteIndexes; pass nil to leave owners unresolved.
func Load(name string, siteIndex map[string]Owner) Engine {
	status := serviceops.ServiceStatus(name)
	snapOps := serviceops.SnapshotSupported(name, false)
	eng := Engine{
		Service:          name,
		Family:           config.FamilyOfName(name),
		Running:          status == "active",
		SupportsSnapshot: snapOps,
	}
	if !eng.Running {
		return eng
	}
	command := serviceops.IntrospectCommand(name)
	if command == "" {
		return eng
	}
	dbs, err := serviceops.ListDatabases(name, command)
	if err != nil {
		eng.Error = err.Error()
		return eng
	}
	for _, db := range dbs {
		entry := Entry{Name: db.Name, SizeBytes: db.SizeBytes, Owner: siteIndex[db.Name]}
		if snapOps {
			if snaps, sErr := serviceops.ListSnapshots(name, db.Name, false); sErr == nil {
				entry.Snapshots = snaps
			}
		}
		eng.Databases = append(eng.Databases, entry)
	}
	return eng
}

// LoadAll builds the whole surface: every installed engine with its databases,
// resolved against one pass over the sites.
func LoadAll() []Engine {
	names := InstalledEngines()
	indexes := SiteIndexes()
	engines := make([]Engine, 0, len(names))
	for _, name := range names {
		engines = append(engines, Load(name, indexes[name]))
	}
	return engines
}
