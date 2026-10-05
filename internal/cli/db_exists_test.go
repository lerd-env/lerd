package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A mysql client that runs but cannot reach the server is the error worth
// reporting; the mariadb fallback missing from a MySQL image must not hide it.
func TestDatabaseExists_ReportsTheClientErrorNotTheMissingFallback(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$3" in
  mysql) echo "mysql: [Warning] Using a password on the command line interface can be insecure." >&2
         echo "ERROR 2002 (HY000): Can't connect to local MySQL server through socket '/var/run/mysqld/mysqld.sock' (2)" >&2
         exit 1 ;;
  *) echo "executable file not found" >&2; exit 127 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "podman"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := databaseExists("mysql", "demo")
	if err == nil {
		t.Fatal("an unreachable server reported no error")
	}
	if !strings.Contains(err.Error(), "Can't connect to local MySQL server") {
		t.Errorf("error = %q, want the client's own message", err)
	}
}

// A client binary the image does not ship is skipped for the next one.
func TestDatabaseExists_FallsBackWhenTheClientIsMissing(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$3" in
  mariadb) [ "$4 $5" = "-h 127.0.0.1" ] || { echo "not over TCP: $*" >&2; exit 1; }
           echo 1 ;;
  *) exit 127 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "podman"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	exists, err := databaseExists("mysql", "demo")
	if err != nil || !exists {
		t.Fatalf("databaseExists = %v, %v; want true through the mariadb client", exists, err)
	}
}
