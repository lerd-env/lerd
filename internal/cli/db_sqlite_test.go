package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// lerd's database commands drive a database service container. A SQLite project
// has none, and its DB_DATABASE is a file path, not a database name. Passing that
// path down to the service layer produced the nonsense
//
//	invalid database name "database/database.sqlite": use letters, digits, underscores and dashes only
//
// which blames the user's database name for what is really "SQLite is not a
// service". Laravel's own default is SQLite, so `lerd new` + `lerd setup --all`
// lands every new user here.
func TestLoadDBEnvRejectsSQLiteWithAnAnswerAboutSQLite(t *testing.T) {
	dir := writeEnvFixture(t, "DB_CONNECTION=sqlite\nDB_DATABASE=database/database.sqlite\n")

	_, err := loadDBEnv(dir)
	if err == nil {
		t.Fatal("loadDBEnv accepted a sqlite project; it has no database service to target")
	}
	msg := err.Error()
	if !strings.Contains(msg, "SQLite") {
		t.Errorf("error does not name SQLite as the reason:\n%s", msg)
	}
	if !strings.Contains(msg, "database/database.sqlite") {
		t.Errorf("error does not name the file it found:\n%s", msg)
	}
	if !strings.Contains(msg, "--service") {
		t.Errorf("error does not point at the way out (--service):\n%s", msg)
	}
	if strings.Contains(msg, "invalid database name") {
		t.Errorf("error still blames the database name:\n%s", msg)
	}
}

// The same guard belongs on the lenient path, which db:shell and db:create use.
func TestLoadDBEnvLenientRejectsSQLite(t *testing.T) {
	dir := writeEnvFixture(t, "DB_CONNECTION=sqlite\nDB_DATABASE=database/database.sqlite\n")

	if _, err := loadDBEnvLenient(dir); err == nil || !strings.Contains(err.Error(), "SQLite") {
		t.Errorf("loadDBEnvLenient did not reject sqlite with a SQLite answer: %v", err)
	}
}

// An explicit --service still wins: someone running a SQLite app who also has a
// mysql service can point the command at it by hand.
func TestExplicitServiceBeatsTheSQLiteGuard(t *testing.T) {
	dir := writeEnvFixture(t, "DB_CONNECTION=sqlite\nDB_DATABASE=database/database.sqlite\n")

	env, err := resolveDB(dir, "mysql", "shop")
	if err != nil {
		t.Fatalf("explicit --service was refused on a sqlite project: %v", err)
	}
	if env.service != "mysql" || env.database != "shop" {
		t.Errorf("got service=%q database=%q, want mysql/shop", env.service, env.database)
	}
}

// A DATABASE_URL naming sqlite reaches the same guard, since that is how some
// frameworks spell the same thing.
func TestLoadDBEnvRejectsSQLiteFromDatabaseURL(t *testing.T) {
	dir := writeEnvFixture(t, "DATABASE_URL=sqlite:///var/data/app.db\n")

	if _, err := loadDBEnv(dir); err == nil || !strings.Contains(err.Error(), "SQLite") {
		t.Errorf("a sqlite DATABASE_URL was not rejected with a SQLite answer: %v", err)
	}
}

// The guard is only useful if the reason survives the resolution chain. It
// first did not: resolveDB caught any loadDBEnv failure and replaced it with
// its own "no DB config found", so the user lost the SQLite explanation and was
// told lerd could not find a database instead of that it had found the wrong
// kind. This asserts through the function every db command actually calls.
func TestResolveDBKeepsTheSQLiteReason(t *testing.T) {
	dir := writeEnvFixture(t, "DB_CONNECTION=sqlite\nDB_DATABASE=database/database.sqlite\n")

	_, err := resolveDB(dir, "", "")
	if err == nil {
		t.Fatal("resolveDB accepted a sqlite project")
	}
	if !errors.Is(err, errSQLiteProject) {
		t.Errorf("resolveDB error does not wrap errSQLiteProject: %v", err)
	}
	if !strings.Contains(err.Error(), "SQLite") {
		t.Errorf("resolveDB flattened the reason away:\n%s", err.Error())
	}
	if strings.Contains(err.Error(), "no DB config found") {
		t.Errorf("resolveDB replaced the reason with the generic message:\n%s", err.Error())
	}
}

// db:shell and db:create go through the lenient path, which used to fall back to
// a mysql default and would have tried to open a shell on a service the project
// does not use.
func TestResolveDBLenientKeepsTheSQLiteReason(t *testing.T) {
	dir := writeEnvFixture(t, "DB_CONNECTION=sqlite\nDB_DATABASE=database/database.sqlite\n")

	if _, err := resolveDBLenient(dir, "", ""); !errors.Is(err, errSQLiteProject) {
		t.Errorf("resolveDBLenient did not surface the sqlite reason: %v", err)
	}
}

// externalDBFixture is a project on the env given, under an isolated config with
// lerd's MySQL published on 3308.
func externalDBFixture(t *testing.T, env string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		if c.Services == nil {
			c.Services = map[string]config.ServiceConfig{}
		}
		sc := c.Services["mysql"]
		sc.Port = 3308
		c.Services["mysql"] = sc
	}); err != nil {
		t.Fatal(err)
	}
	return writeEnvFixture(t, env)
}

// Every db command runs inside lerd's container, so one that read a site on an
// outside server would dump, or overwrite, any database of the same name there.
func TestResolveDBRefusesADatabaseLerdDoesNotServe(t *testing.T) {
	for name, env := range map[string]string{
		"the machine, from a container": "DB_CONNECTION=mysql\nDB_HOST=host.containers.internal\nDB_PORT=3306\nDB_DATABASE=shop\n",
		"another machine":               "DB_CONNECTION=mysql\nDB_HOST=db.example.com\nDB_DATABASE=shop\n",
		"loopback on another port":      "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\nDB_DATABASE=shop\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := externalDBFixture(t, env)
			if _, err := resolveDB(dir, "", ""); err == nil || !strings.Contains(err.Error(), "--service") {
				t.Errorf("resolveDB: %v", err)
			}
			if _, err := resolveDBLenient(dir, "", ""); err == nil || !strings.Contains(err.Error(), "--service") {
				t.Errorf("resolveDBLenient: %v", err)
			}
		})
	}
}

func TestResolveDBRefusesAServiceMarkedExternal(t *testing.T) {
	dir := externalDBFixture(t, "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3308\nDB_DATABASE=shop\n")
	if err := os.WriteFile(filepath.Join(dir, ".env.lerd_override"), []byte("LERD_EXTERNAL_SERVICES=mysql\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveDB(dir, "", ""); err == nil || !strings.Contains(err.Error(), "--service") {
		t.Errorf("resolveDB: %v", err)
	}
}

func TestResolveDBKeepsWorkingOnLerdsOwnDatabase(t *testing.T) {
	for name, env := range map[string]string{
		"the container":         "DB_CONNECTION=mysql\nDB_HOST=lerd-mysql\nDB_DATABASE=shop\n",
		"the published port":    "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3308\nDB_DATABASE=shop\n",
		"loopback with no port": "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_DATABASE=shop\n",
		"no host at all":        "DB_CONNECTION=mysql\nDB_DATABASE=shop\n",
	} {
		t.Run(name, func(t *testing.T) {
			env, err := resolveDB(externalDBFixture(t, env), "", "")
			if err != nil || env.service != "mysql" || env.database != "shop" {
				t.Errorf("got %+v, %v", env, err)
			}
		})
	}
}

// Naming the service is the user saying which container they mean.
func TestResolveDBLetsAnExplicitServiceThrough(t *testing.T) {
	dir := externalDBFixture(t, "DB_CONNECTION=mysql\nDB_HOST=host.containers.internal\nDB_DATABASE=shop\n")
	env, err := resolveDB(dir, "mysql", "shop")
	if err != nil || env.service != "mysql" {
		t.Errorf("got %+v, %v", env, err)
	}
}

// A site on a server someone added stays there when this machine lacks that
// service: falling back would rewire it to lerd's own MySQL as root.
func TestMissingExternalService(t *testing.T) {
	onServer := "DB_CONNECTION=mysql\nDB_HOST=host.containers.internal\nDB_DATABASE=shop\n"
	for name, tc := range map[string]struct {
		env, yaml string
		want      string
	}{
		"a service nobody here has": {onServer, "services:\n  - dbngin\n", "dbngin"},
		"a lerd service":            {onServer, "services:\n  - mysql\n  - redis\n", ""},
		"nothing listed":            {onServer, "", ""},
		"on lerd's own database":    {"DB_CONNECTION=mysql\nDB_HOST=lerd-mysql\nDB_DATABASE=shop\n", "services:\n  - dbngin\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			dir := externalDBFixture(t, tc.env)
			if tc.yaml != "" {
				if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(tc.yaml), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := missingExternalService(dir); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// externalServiceFixture is a project that chose an external server.
func externalServiceFixture(t *testing.T, family string) string {
	t.Helper()
	dir := externalDBFixture(t, "DB_CONNECTION=mysql\nDB_HOST=host.containers.internal\nDB_PORT=3306\nDB_DATABASE=shop\n")
	svc := &config.CustomService{Name: "dbngin", Image: "img", Family: family, External: &config.ExternalDB{Host: "host.containers.internal", Port: 3306, User: "me"}}
	if err := config.SaveCustomService(svc); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("services:\n  - dbngin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A server the site chose is reached through its own client, which is what makes
// every database command work on it.
func TestResolveDBUsesTheExternalServiceTheSiteChose(t *testing.T) {
	dir := externalServiceFixture(t, "mariadb")
	for name, resolve := range map[string]func() (*dbEnv, error){
		"strict":  func() (*dbEnv, error) { return resolveDB(dir, "", "") },
		"lenient": func() (*dbEnv, error) { return resolveDBLenient(dir, "", "") },
	} {
		t.Run(name, func(t *testing.T) {
			env, err := resolve()
			if err != nil || env.service != "dbngin" || env.database != "shop" || !env.external {
				t.Fatalf("got %+v, %v", env, err)
			}
		})
	}
	if env, err := resolveDB(dir, "mysql", "other"); err != nil || env.service != "mysql" || env.external {
		t.Errorf("an explicit service still wins: %+v, %v", env, err)
	}
}

func TestDbShellArgs_ExternalReadsTheClientFile(t *testing.T) {
	for name, env := range map[string]*dbEnv{
		"mysql":    {service: "dbngin", connection: "mysql", database: "shop", external: true},
		"postgres": {service: "dbngin", connection: "pgsql", database: "shop", external: true},
	} {
		t.Run(name, func(t *testing.T) {
			args := strings.Join(dbShellArgs(env, env.connection, false), " ")
			if strings.Contains(args, "127.0.0.1") || strings.Contains(args, "-u") || strings.Contains(args, "-U") || strings.Contains(args, "-p") && !strings.Contains(args, "exec") {
				t.Errorf("lerd's own server leaked in: %s", args)
			}
			if !strings.HasSuffix(args, "shop") {
				t.Errorf("args = %s", args)
			}
		})
	}
}
