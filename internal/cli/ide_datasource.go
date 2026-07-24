package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/ide"
	"github.com/geodro/lerd/internal/podman"
)

// jdbcDialect maps a database family onto what a JetBrains IDE calls its
// driver. It is the last thing here that knows one engine from another, and
// belongs in the service store alongside the rest of the per-engine data (see
// the introspect and extensions fields) once that schema grows a place for it.
var jdbcDialect = map[string]struct{ driver, class, scheme string }{
	"postgres": {"postgresql", "org.postgresql.Driver", "postgresql"},
	"mysql":    {"mysql", "com.mysql.cj.jdbc.Driver", "mysql"},
	"mariadb":  {"mariadb", "org.mariadb.jdbc.Driver", "mariadb"},
}

// syncIDEDataSource points a JetBrains project at the site's database, using
// coordinates that work from the machine rather than the ones in the site's env
// file, which name the container and are unusable outside the network. Silent
// when the project is not a JetBrains one or the engine has no JDBC dialect.
// ideSyncResult says what a sync did, and why when it did nothing, so a command
// can report the actual reason instead of listing the possibilities.
type ideSyncResult struct {
	outcome  ide.Outcome
	database string
	reason   string // set when there was nothing lerd could point at
}

func (r ideSyncResult) wrote() bool { return r.outcome == ide.Written && r.reason == "" }

func syncIDEDataSource(siteRoot string) ideSyncResult {
	if !config.IDEDataSourceEnabled() {
		return ideSyncResult{reason: "ide_data_source is off in config.yaml"}
	}
	env, err := resolveDB(siteRoot, "", "")
	if err != nil || env.database == "" {
		return ideSyncResult{reason: "no database is configured for this project"}
	}
	ds, ok := ideDataSourceFor(env)
	if !ok {
		return ideSyncResult{reason: "lerd has no IDE driver for " + env.connection + " databases"}
	}
	outcomes, err := ide.Sync(siteRoot, []ide.DataSource{ds})
	if err != nil {
		return ideSyncResult{reason: err.Error()}
	}
	return ideSyncResult{outcome: outcomes[0], database: env.database}
}

// removeIDEDataSource drops lerd's entry when a site stops being lerd's.
func removeIDEDataSource(siteRoot string) {
	_, _ = ide.Remove(siteRoot)
}

// ideDataSourceFor builds the connection as an IDE stores it. The password is
// carried in the URL because JetBrains keeps secrets in its own credential
// store, which lerd cannot write, and it is the fixed local one that the site's
// env file already spells out next to it.
func ideDataSourceFor(env *dbEnv) (ide.DataSource, bool) {
	family := config.FamilyOfName(env.service)
	dialect, ok := jdbcDialect[family]
	if !ok {
		return ide.DataSource{}, false
	}
	port := hostPortForService(env.service)
	if port == 0 || env.database == "" {
		return ide.DataSource{}, false
	}
	return ide.DataSource{
		Key:    env.database,
		Name:   env.database + " (lerd)",
		Driver: dialect.driver,
		Class:  dialect.class,
		URL:    fmt.Sprintf("jdbc:%s://127.0.0.1:%d/%s", dialect.scheme, port, env.database),
		User:   env.username,
	}, true
}

// hostPortForService returns the port the engine answers on from the machine,
// read fresh every time: a second engine of the same family is published on a
// shifted port, and that shift can happen long after a site was linked.
func hostPortForService(service string) int {
	if p := config.ServicePublishedPort(service); p > 0 {
		return p
	}
	if custom, err := config.LoadCustomService(service); err == nil && len(custom.Ports) > 0 {
		if p := podman.PrimaryHostPort(custom.Ports); p > 0 {
			return p
		}
	}
	return podman.PrimaryHostPort(config.PresetPorts(service))
}
