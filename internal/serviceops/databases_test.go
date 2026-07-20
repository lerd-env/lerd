package serviceops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDatabaseRows(t *testing.T) {
	out := "laravel_app\t25690112\nwordpress\t18979224\nempty\t0\n"
	got := parseDatabaseRows([]byte(out))
	want := []DatabaseInfo{
		{Name: "laravel_app", SizeBytes: 25690112},
		{Name: "wordpress", SizeBytes: 18979224},
		{Name: "empty", SizeBytes: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseDatabaseRowsSkipsBlankAndTrailing(t *testing.T) {
	// Blank lines, a trailing carriage return, and stray whitespace must not
	// produce phantom databases or corrupt names.
	out := "\n  app  \t 1024 \r\n\n"
	got := parseDatabaseRows([]byte(out))
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(got), got)
	}
	if got[0].Name != "app" || got[0].SizeBytes != 1024 {
		t.Errorf("row = %+v, want {app 1024}", got[0])
	}
}

func TestParseDatabaseRowsNameOnly(t *testing.T) {
	// A query that emits only names (no size column) still lists databases.
	got := parseDatabaseRows([]byte("analytics\nmetrics\n"))
	if len(got) != 2 || got[0].Name != "analytics" || got[0].SizeBytes != 0 {
		t.Fatalf("unexpected parse: %+v", got)
	}
}

// writeCustomService drops a service YAML into an isolated config dir so
// introspect resolution can be exercised without touching the real install.
func writeCustomService(t *testing.T, name, body string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "lerd", "services")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestIntrospectCommandFromCustomService(t *testing.T) {
	writeCustomService(t, "myengine", "name: myengine\nimage: example/engine:1\nintrospect:\n  list_databases: echo hi\n")
	if got := IntrospectCommand("myengine"); got != "echo hi" {
		t.Fatalf("got %q, want %q", got, "echo hi")
	}
}

func TestIntrospectCommandFallsBackToPreset(t *testing.T) {
	// An engine installed before the introspect field existed carries no
	// introspect block of its own, so the preset it came from supplies it.
	writeCustomService(t, "mysql", "name: mysql\nimage: docker.io/library/mysql:8\npreset: mysql\n")
	if IntrospectCommand("mysql") == "" {
		t.Fatal("want the bundled mysql preset command, got none")
	}
}

func TestIntrospectCommandUnknownEngine(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got := IntrospectCommand("nosuchengine"); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestListDatabasesEmptyCommand(t *testing.T) {
	// An engine with no introspect command yields no databases and no error,
	// never touching a container.
	dbs, err := ListDatabases("mysql", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dbs != nil {
		t.Fatalf("want nil, got %+v", dbs)
	}
}
