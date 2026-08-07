package config

import (
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/envfile"
)

// EnvFileFor resolves the file a project keeps its configuration in, and the
// format it is written in, both declared by the framework definition. Laravel's
// dotenv .env is only the default: WordPress keeps PHP constants in
// wp-config.php, Magento a PHP array in app/etc/env.php, Symfony its real values
// in .env.local. Anything that wants to read a project's configuration goes
// through this rather than joining ".env" onto the site path.
func EnvFileFor(projectDir string) (file, format string) {
	fw := frameworkFor(projectDir)
	if fw == nil {
		return ".env", "dotenv"
	}
	return fw.Env.Resolve(projectDir)
}

// DBEnvBinding describes how a project's env file addresses its database: the
// file and format holding it, and the keys naming the host container and the
// database itself. It is what a writer aims at; readers want DBTargetFor.
type DBEnvBinding struct {
	File    string
	Format  string
	HostKey string
	NameKey string
}

// DBTarget is a database a project points at: the lerd service holding it and
// the name of the database inside that service.
type DBTarget struct {
	Service  string
	Database string
}

// dbServiceBinding is one declared database service and how the framework
// addresses it. urlKey stands in for the host/name pair when the service is
// wired through a single connection string (Symfony's DATABASE_URL), which
// carries both.
type dbServiceBinding struct {
	DBEnvBinding
	service string
	detect  []FrameworkServiceDetect
	urlKey  string
}

// defaultDBEnvBinding is the shape Laravel gave everyone, and what a project
// whose framework declares no database service falls back to.
func defaultDBEnvBinding() DBEnvBinding {
	return DBEnvBinding{File: ".env", Format: "dotenv", HostKey: "DB_HOST", NameKey: "DB_DATABASE"}
}

// frameworkFor returns the framework definition governing a project directory,
// or nil when none is detected.
func frameworkFor(projectDir string) *Framework {
	name, ok := DetectFrameworkForDir(projectDir)
	if !ok {
		return nil
	}
	fw, ok := GetFrameworkForDir(name, projectDir)
	if !ok {
		return nil
	}
	return fw
}

// dbEnvBindings derives from the framework declaration how each database
// service it supports is addressed, ordered so the primary one is picked the
// same way every run, along with the env file and format they all read from.
// The keys come out of the service vars: the one set to {{site}} names the
// database, the one set to a lerd-<service> container names the host, and a var
// carrying both inside a URL is the DSN.
func dbEnvBindings(projectDir string) (bindings []dbServiceBinding, file, format string) {
	file, format = ".env", "dotenv"
	fw := frameworkFor(projectDir)
	if fw == nil {
		return nil, file, format
	}
	file, format = fw.Env.Resolve(projectDir)
	if len(fw.Env.Services) == 0 {
		return nil, file, format
	}
	var out []dbServiceBinding
	for name, def := range fw.Env.Services {
		if name == "sqlite" || !IsDBServiceName(name) {
			continue
		}
		b := dbServiceBinding{
			DBEnvBinding: DBEnvBinding{File: file, Format: format},
			service:      name,
			detect:       def.Detect,
		}
		for _, kv := range def.Vars {
			key, val, found := strings.Cut(kv, "=")
			if !found {
				continue
			}
			switch {
			case val == "{{site}}":
				b.NameKey = key
			case strings.HasPrefix(val, "lerd-"):
				b.HostKey = key
			case strings.Contains(val, "{{site}}") && strings.Contains(val, "lerd-"):
				b.urlKey = key
			}
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := dbServiceRank(out[i].service), dbServiceRank(out[j].service)
		if ri != rj {
			return ri < rj
		}
		return out[i].service < out[j].service
	})
	return out, file, format
}

// dbServiceRank orders the database services a framework may declare, so a
// definition supporting several of them resolves to the same primary every run.
func dbServiceRank(name string) int {
	families := []string{"mysql", "mariadb", "postgres", "mongo"}
	for i, family := range families {
		if FamilyOfName(name) == family {
			return i
		}
	}
	return len(families)
}

// DBEnvBindingFor returns how a project addresses the database it uses: the
// declared service whose detect rules match its env file wins, then the first
// database service the framework declares, and finally the default shape, so a
// writer always has a file and a pair of keys to aim at. A framework that
// encodes its database in a DSN has no standalone name key, so it too yields
// the default and the caller reports the site as unmanaged.
func DBEnvBindingFor(projectDir string) DBEnvBinding {
	bindings, file, format := dbEnvBindings(projectDir)
	if len(bindings) == 0 {
		return defaultDBEnvBinding()
	}
	vals := envfile.Values(filepath.Join(projectDir, file), format)
	var first *DBEnvBinding
	for _, b := range bindings {
		if b.NameKey == "" || b.HostKey == "" {
			continue
		}
		if serviceDetectedIn(b.detect, vals) {
			return b.DBEnvBinding
		}
		if first == nil {
			binding := b.DBEnvBinding
			first = &binding
		}
	}
	if first != nil {
		return *first
	}
	return defaultDBEnvBinding()
}

// DBTargetFor returns the database a project uses as its framework declares it:
// read from the file the definition names, in the format it names, at the keys
// it names. ok is false when no framework declares a database service the
// project's env file matches, leaving the caller free to fall back to its own
// inference.
func DBTargetFor(projectDir string) (DBTarget, bool) {
	bindings, file, format := dbEnvBindings(projectDir)
	vals := envfile.Values(filepath.Join(projectDir, file), format)
	for _, t := range declaredDBTargets(projectDir, bindings, vals) {
		return t, true
	}
	return DBTarget{}, false
}

// DBTargetsFor returns every lerd-managed database a project points at: the
// ones its framework declares, plus the default keys and any connection string
// aimed at a lerd engine, so a project whose framework lerd doesn't recognise
// still shows as the owner of its database.
func DBTargetsFor(projectDir string) []DBTarget {
	bindings, file, format := dbEnvBindings(projectDir)
	vals := envfile.Values(filepath.Join(projectDir, file), format)

	var out []DBTarget
	claimed := map[string]bool{}
	add := func(t DBTarget) {
		if t.Service == "" || t.Database == "" || claimed[t.Service] {
			return
		}
		claimed[t.Service] = true
		out = append(out, t)
	}
	for _, t := range declaredDBTargets(projectDir, bindings, vals) {
		add(t)
	}
	add(DBTarget{
		Service:  dbServiceFor(projectDir, vals["DB_HOST"], ""),
		Database: vals["DB_DATABASE"],
	})
	for _, t := range dsnTargets(vals) {
		add(t)
	}
	return out
}

// declaredDBTargets resolves the databases the framework declaration accounts
// for, in the order the declaration ranks them.
func declaredDBTargets(projectDir string, bindings []dbServiceBinding, vals map[string]string) []DBTarget {
	var out []DBTarget
	for _, b := range bindings {
		if !serviceDetectedIn(b.detect, vals) {
			continue
		}
		t := DBTarget{}
		switch {
		case b.urlKey != "":
			t = dsnTarget(vals[b.urlKey])
		case b.NameKey != "" && b.HostKey != "":
			t = DBTarget{
				Service:  dbServiceFor(projectDir, vals[b.HostKey], b.service),
				Database: vals[b.NameKey],
			}
		default:
			// A definition that detects a service without publishing its vars
			// still names a real database somewhere in the file it points at.
			t = declaredFallbackTarget(projectDir, b.service, vals)
		}
		if t.Service != "" && t.Database != "" {
			out = append(out, t)
		}
	}
	return out
}

// declaredFallbackTarget resolves a detected service whose declaration carries
// no vars, from the default keys and then from any connection string in the
// file aimed at that service's family.
func declaredFallbackTarget(projectDir, service string, vals map[string]string) DBTarget {
	if db := vals["DB_DATABASE"]; db != "" {
		return DBTarget{Service: dbServiceFor(projectDir, vals["DB_HOST"], service), Database: db}
	}
	family := FamilyOfName(service)
	for _, t := range dsnTargets(vals) {
		if FamilyOfName(t.Service) == family {
			return t
		}
	}
	return DBTarget{}
}

// serviceDetectedIn reports whether any of a declared service's detect rules
// matches the project's env values. A declaration carrying no rules has nothing
// to rule it out, so it is taken as applying and the values read at its keys
// decide whether it resolves to a database at all.
func serviceDetectedIn(detect []FrameworkServiceDetect, vals map[string]string) bool {
	if len(detect) == 0 {
		return true
	}
	for _, rule := range detect {
		val, exists := vals[rule.Key]
		if !exists {
			continue
		}
		if rule.ValuePrefix == "" || strings.HasPrefix(val, rule.ValuePrefix) {
			return true
		}
	}
	return false
}

// DBServiceFor returns the lerd service backing a project's database, given the
// host value read from its env file. Empty when the database is not one lerd
// manages.
func DBServiceFor(projectDir, host string) string {
	return dbServiceFor(projectDir, host, "")
}

// dbServiceFor returns the lerd service backing a database, from the host value
// the project's env file holds. Container and PHP sites name it directly
// (lerd-postgres-18 -> postgres-18); a host-proxy site rewrites the host to
// loopback, so the service is recovered from the database entry in its
// .lerd.yaml. An empty host falls back to the service the framework declared
// the block for, and a host lerd doesn't manage resolves to nothing, because
// that database is not ours.
func dbServiceFor(projectDir, host, declared string) string {
	host = strings.TrimSpace(host)
	if svc, ok := strings.CutPrefix(host, "lerd-"); ok && svc != "" {
		return strings.TrimSuffix(strings.Split(svc, ":")[0], "/")
	}
	if proj, err := LoadProjectConfig(projectDir); err == nil {
		for _, svc := range proj.Services {
			if IsDBServiceName(svc.Name) && svc.Name != "sqlite" {
				return svc.Name
			}
		}
	}
	if host == "" {
		return declared
	}
	return ""
}

// dsnTargets returns the databases the values point at through a connection
// string, one per service. Keys are visited in sorted order: a project with
// more than one DSN against the same engine would otherwise be attributed to a
// different database on every read, since Go randomises map iteration.
func dsnTargets(vals map[string]string) []DBTarget {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []DBTarget
	seen := map[string]bool{}
	for _, k := range keys {
		t := dsnTarget(vals[k])
		if t.Service == "" || t.Database == "" || seen[t.Service] {
			continue
		}
		seen[t.Service] = true
		out = append(out, t)
	}
	return out
}

// dsnTarget parses a connection string aimed at a lerd database engine
// (mongodb://root:lerd@lerd-mongo:27017/jobs) into its service and database.
// A value that is not such a URL yields the zero target.
func dsnTarget(value string) DBTarget {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, "lerd-") {
		return DBTarget{}
	}
	u, err := url.Parse(value)
	if err != nil {
		return DBTarget{}
	}
	service, ok := strings.CutPrefix(u.Hostname(), "lerd-")
	if !ok || service == "" || !dsnServiceHoldsDatabases(service) {
		return DBTarget{}
	}
	db := strings.TrimPrefix(u.Path, "/")
	if db == "" || strings.Contains(db, "/") {
		return DBTarget{}
	}
	return DBTarget{Service: service, Database: db}
}

// dsnServiceHoldsDatabases keeps a cache or queue connection string out of the
// database walk: redis://lerd-redis:6379/0 ends in a database index, not a
// name. An engine whose preset isn't installed has no family to judge it by, so
// it stays in and the caller matches it against the engines it actually has.
func dsnServiceHoldsDatabases(service string) bool {
	if service == "sqlite" {
		return false
	}
	if FamilyOfName(service) == "" {
		return true
	}
	return IsDBServiceName(service)
}
