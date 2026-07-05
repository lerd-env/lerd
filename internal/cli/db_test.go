package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDbImportCmdMySQLPasswordNotInArgs(t *testing.T) {
	env := &dbEnv{
		connection: "mysql",
		database:   "testdb",
		username:   "root",
		password:   "secret123",
	}
	cmd, err := dbImportCmd(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range cmd.Args {
		if strings.Contains(arg, "secret123") && !strings.HasPrefix(arg, "MYSQL_PWD=") {
			t.Errorf("password leaked in command arg: %q", arg)
		}
	}
	// Verify password is passed via env, not -p flag
	for _, arg := range cmd.Args {
		if strings.HasPrefix(arg, "-psecret123") || strings.HasPrefix(arg, "-p=secret123") {
			t.Errorf("password passed via -p flag: %q", arg)
		}
	}
}

func TestDbExportCmdMySQLPasswordNotInArgs(t *testing.T) {
	env := &dbEnv{
		connection: "mysql",
		database:   "testdb",
		username:   "root",
		password:   "secret123",
	}
	cmd, err := dbExportCmd(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range cmd.Args {
		if strings.Contains(arg, "secret123") && !strings.HasPrefix(arg, "MYSQL_PWD=") {
			t.Errorf("password leaked in command arg: %q", arg)
		}
	}
}

func TestDbCmdPostgresUsesEnv(t *testing.T) {
	env := &dbEnv{
		connection: "pgsql",
		database:   "testdb",
		username:   "postgres",
		password:   "secret123",
	}
	cmd, err := dbImportCmd(env)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, arg := range cmd.Args {
		if arg == "PGPASSWORD=secret123" {
			found = true
		}
	}
	if !found {
		t.Error("expected PGPASSWORD env var in postgres command")
	}
}

func TestDbExportCmdMariaDBBinaryFallback(t *testing.T) {
	env := &dbEnv{connection: "mariadb", database: "shop", username: "root", password: "lerd"}
	cmd, err := dbExportCmd(env)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "command -v mysqldump || command -v mariadb-dump") {
		t.Errorf("expected mariadb-dump fallback, got: %q", joined)
	}
	if !strings.Contains(joined, "shop") {
		t.Errorf("expected database name in command, got: %q", joined)
	}
}

func TestDbImportCmdMariaDBBinaryFallback(t *testing.T) {
	env := &dbEnv{connection: "mysql", database: "shop", username: "root", password: "lerd"}
	cmd, err := dbImportCmd(env)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "command -v mysql || command -v mariadb") {
		t.Errorf("expected mariadb client fallback, got: %q", joined)
	}
}

func TestDbCmdUnsupportedConnection(t *testing.T) {
	env := &dbEnv{connection: "sqlite"}
	_, err := dbImportCmd(env)
	if err == nil {
		t.Error("expected error for unsupported connection")
	}
	_, err = dbExportCmd(env)
	if err == nil {
		t.Error("expected error for unsupported connection")
	}
}

func TestLerdServiceFromHost(t *testing.T) {
	cases := map[string]string{
		"lerd-mariadb-11-8": "mariadb-11-8",
		"lerd-mysql":        "mysql",
		"lerd-postgres-18":  "postgres-18",
		"  lerd-mysql ":     "mysql",
		"127.0.0.1":         "",
		"db.example.com":    "",
		"":                  "",
		"lerd-":             "",
	}
	for host, want := range cases {
		if got := lerdServiceFromHost(host); got != want {
			t.Errorf("lerdServiceFromHost(%q) = %q, want %q", host, got, want)
		}
	}
}

func writeEnvFixture(t *testing.T, lines string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(lines), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadDBEnvTargetsActualServiceFromHost(t *testing.T) {
	// A mariadb-backed site: mysql dialect, but the container is the mariadb one.
	dir := writeEnvFixture(t, "DB_CONNECTION=mysql\nDB_HOST=lerd-mariadb-11-8\nDB_DATABASE=shop\n")
	env, err := loadDBEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if env.service != "mariadb-11-8" {
		t.Errorf("service = %q, want mariadb-11-8", env.service)
	}
	if env.connection != "mysql" {
		t.Errorf("connection = %q, want mysql", env.connection)
	}
}

func TestLoadDBEnvCanonicalHostUnchanged(t *testing.T) {
	dir := writeEnvFixture(t, "DB_CONNECTION=pgsql\nDB_HOST=lerd-postgres\nDB_DATABASE=shop\n")
	env, err := loadDBEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if env.service != "postgres" {
		t.Errorf("service = %q, want postgres", env.service)
	}
}

func TestLoadDBEnvNonLerdHostFallsBackToCanonical(t *testing.T) {
	dir := writeEnvFixture(t, "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_DATABASE=shop\n")
	env, err := loadDBEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if env.service != "mysql" {
		t.Errorf("service = %q, want mysql (canonical fallback)", env.service)
	}
}
