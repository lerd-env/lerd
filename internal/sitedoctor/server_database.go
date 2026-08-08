package sitedoctor

import (
	"fmt"
	"strings"

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
	return func() { listDatabases = prev }
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
func checkServerDatabase(path string, fw *config.Framework) (Check, bool) {
	targets := config.DBTargetsFor(path)
	if len(targets) == 0 {
		// No lerd-run database: a file database, an external server, or nothing
		// configured. None of those is this check's to judge.
		return Check{}, false
	}
	checked := false
	for _, t := range targets {
		names, err := listDatabases(t.Service)
		if err != nil {
			// The engine is down or unreachable. Reporting that as a missing
			// schema would send the user to create a database that may exist.
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
			return Check{Name: "server_database", Status: StatusFail, Fix: migrateFix(fw),
				Detail: fmt.Sprintf("Database %q does not exist on %s — create it with lerd db:create %s, then run migrations.", t.Database, t.Service, t.Database)}, true
		}
	}
	if !checked {
		// Nothing could be asked, so there is nothing to report either way.
		return Check{}, false
	}
	return Check{Name: "server_database", Status: StatusOK}, true
}
