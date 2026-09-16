package cli

import (
	"errors"
	"strings"
	"testing"
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
