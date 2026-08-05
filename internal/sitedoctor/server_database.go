package sitedoctor

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/serviceops"
)

// serverDatabaseFamilies are the env driver names that mean the site's data
// lives in an engine lerd runs, rather than in a file next to the code.
var serverDatabaseFamilies = map[string]bool{
	"mysql": true, "mariadb": true, "pgsql": true, "postgres": true, "postgresql": true,
}

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

// checkServerDatabase fails when the env file points at a MySQL or Postgres
// schema that does not exist on the service. Without it such a site 500s on
// every request while the doctor reports nothing wrong: the framework's own
// migration check cannot reach the app either, so it degrades to "couldn't run"
// and is not counted as a failure.
func checkServerDatabase(_ string, envPath string, fw *config.Framework) (Check, bool) {
	driver := strings.ToLower(strings.TrimSpace(envfile.ReadKey(envPath, "DB_CONNECTION")))
	if !serverDatabaseFamilies[driver] {
		return Check{}, false
	}
	dbName := strings.TrimSpace(envfile.ReadKey(envPath, "DB_DATABASE"))
	host := strings.TrimSpace(envfile.ReadKey(envPath, "DB_HOST"))
	service := strings.TrimPrefix(host, "lerd-")
	if dbName == "" || service == "" || !strings.HasPrefix(host, "lerd-") {
		// An external database, or one lerd does not run, is not ours to judge.
		return Check{}, false
	}

	names, err := listDatabases(service)
	if err != nil {
		// The engine is down or unreachable. Reporting that as a missing schema
		// would send the user to create a database that may already exist.
		return Check{}, false
	}
	for _, n := range names {
		if strings.EqualFold(n, dbName) {
			return Check{Name: "server_database", Status: StatusOK}, true
		}
	}

	fix := ""
	if frameworkHasCommand(fw, sqliteFixCommand) {
		fix = sqliteFixCommand
	}
	return Check{Name: "server_database", Status: StatusFail, Fix: fix,
		Detail: fmt.Sprintf("Database %q does not exist on %s — create it with lerd db:create %s, then run migrations.", dbName, service, dbName)}, true
}
