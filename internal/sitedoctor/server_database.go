package sitedoctor

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

// listDatabases is the engine-agnostic lookup, hooked so tests can drive the
// check without a running container. The command itself comes from the service
// preset, so no engine SQL is hardcoded here.
var listDatabases = func(service string) ([]string, error) {
	infos, err := serviceops.ListDatabases(service, serviceops.IntrospectCommand(service))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(infos))
	for _, db := range infos {
		names = append(names, db.Name)
	}
	return names, nil
}

// stubDatabaseLister swaps the lookup for a test and returns the restore func.
func stubDatabaseLister(fn func(string) ([]string, error)) func() {
	prev := listDatabases
	listDatabases = fn
	ForgetDatabases("")
	return func() {
		listDatabases = prev
		ForgetDatabases("")
	}
}

// dbListTTL bounds how long one engine's database list is reused. `lerd doctor`
// sweeps every site, and without this each one pays its own container exec to
// ask the same engine the same question.
const dbListTTL = 5 * time.Second

type dbListEntry struct {
	names []string
	err   error
	at    time.Time
}

var dbListCache = struct {
	sync.Mutex
	entries map[string]dbListEntry
}{entries: map[string]dbListEntry{}}

// cachedDatabases is listDatabases with the recent answer reused. One lock
// covers every engine and is held across the lookup on purpose: the sweep's
// callers wait for a running exec instead of each starting their own, and a
// machine runs one or two engines, so serialising them costs a single extra
// exec on a cold cache and saves one per site after that.
func cachedDatabases(service string) ([]string, error) {
	dbListCache.Lock()
	defer dbListCache.Unlock()
	if e, ok := dbListCache.entries[service]; ok && time.Since(e.at) < dbListTTL {
		return e.names, e.err
	}
	names, err := listDatabases(service)
	dbListCache.entries[service] = dbListEntry{names: names, err: err, at: time.Now()}
	return names, err
}

// ForgetDatabases drops one engine's cached list, or every engine's when
// service is empty. Anything that creates or removes a database has to call it:
// the cache exists so a sweep does not ask the same engine once per site, and
// the re-check that follows a fix runs well inside that window, so without this
// it reads the list from before the create and reports the database it just
// made as still missing.
func ForgetDatabases(service string) {
	dbListCache.Lock()
	defer dbListCache.Unlock()
	if service == "" {
		dbListCache.entries = map[string]dbListEntry{}
		return
	}
	delete(dbListCache.entries, service)
}

// checkServerDatabase fails when the site's database does not exist on the
// engine it points at. Without it such a site 500s on every request while the
// doctor reports nothing wrong: the framework's own migration check cannot reach
// the app either, so it degrades to "couldn't run" and is not counted.
//
// Which database on which service is read from the framework declaration, so a
// project keeping its configuration in a PHP settings file or behind a DSN is
// checked like any other. It used to read DB_CONNECTION, DB_HOST and DB_DATABASE
// by name, which are Laravel's, so the frameworks least likely to be wired that
// way were the ones it could not check.
//
// The fix creates the database rather than migrating it: migrations fail against
// a database the engine does not hold, so the schema has to exist first, and the
// migrate button returns on the re-check that follows.
func checkServerDatabase(path string) (Check, bool) {
	if gone := uninstalledDBServices(path); len(gone) > 0 {
		hints := make([]string, 0, len(gone))
		for _, name := range gone {
			hints = append(hints, "lerd service start "+name)
		}
		return Check{Name: "server_database", Status: StatusFail,
			Detail: fmt.Sprintf("The site's database is on %s, which lerd has not installed. Install it with %s, then run migrations.",
				strings.Join(gone, " and "), strings.Join(hints, " and "))}, true
	}
	missing, checked := missingDatabases(path)
	if !checked {
		// Either nothing could be asked of an engine, or the project points at no
		// lerd-run database at all: a file database, an external server, or
		// nothing configured. None of those is this check's to judge.
		return Check{}, false
	}
	if len(missing) == 0 {
		return Check{Name: "server_database", Status: StatusOK}, true
	}
	named := make([]string, 0, len(missing))
	for _, t := range missing {
		named = append(named, fmt.Sprintf("%q on %s", t.Database, t.Service))
	}
	return Check{Name: "server_database", Status: StatusFail, Fix: FixCreateDatabase,
		Detail: fmt.Sprintf("%s %s %s not exist. Create %s, then run migrations.",
			Plural(len(missing), "Database", "Databases"), strings.Join(named, ", "),
			Plural(len(missing), "does", "do"), Plural(len(missing), "it", "them"))}, true
}

// uninstalledDBServices returns the lerd services the project's databases sit
// on that could not be asked for their databases because this machine does not
// have them. One installed but not answering is not judged.
func uninstalledDBServices(path string) []string {
	var gone []string
	for _, t := range config.DBTargetsFor(path) {
		if _, err := cachedDatabases(t.Service); err != nil && !serviceInstalledFn(t.Service) {
			gone = append(gone, t.Service)
		}
	}
	return gone
}

// MissingDatabases returns the lerd-managed databases a project points at that
// their engine does not hold. Exported so the fix works the set out again
// rather than trusting the client, the same way the service fixes do.
func MissingDatabases(path string) []config.DBTarget {
	missing, _ := missingDatabases(path)
	return missing
}

// missingDatabases pairs the set with whether any engine could be asked at all:
// an unreachable one leaves the site unjudged rather than reported as missing a
// schema that may well exist.
func missingDatabases(path string) (missing []config.DBTarget, checked bool) {
	for _, t := range config.DBTargetsFor(path) {
		names, err := cachedDatabases(t.Service)
		if err != nil {
			continue
		}
		checked = true
		found := false
		for _, n := range names {
			if strings.EqualFold(n, t.Database) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, t)
		}
	}
	return missing, checked
}
