package serviceops

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
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

// bundledDatabasesAction reads a declared databases action straight off a
// bundled preset, bypassing custom-service resolution so the test never reads
// the developer's real install.
func bundledDatabasesAction(t *testing.T, preset, action string) string {
	t.Helper()
	p, err := config.LoadPreset(preset)
	if err != nil {
		t.Fatalf("loading the %s preset: %v", preset, err)
	}
	spec := p.Introspect.DatabasesEntity()
	if spec == nil {
		t.Fatalf("the %s preset declares no databases entity", preset)
	}
	act, ok := spec.Actions[action]
	if !ok {
		t.Fatalf("the %s preset declares no %s action", preset, action)
	}
	return act.Exec
}

// A dump has to load back over a database that already has the objects, or a
// colleague importing it drowns in "already exists". mysqldump drops each table
// on its own; pg_dump only does it when asked.
func TestDeclaredExportDropsBeforeCreating(t *testing.T) {
	pg := bundledDatabasesAction(t, "postgres", "export")
	for _, flag := range []string{"--clean", "--if-exists"} {
		if !strings.Contains(pg, flag) {
			t.Errorf("postgres export missing %s: %q", flag, pg)
		}
	}
	my := bundledDatabasesAction(t, "mysql", "export")
	if strings.Contains(my, "--skip-add-drop-table") {
		t.Errorf("mysql export disabled its own drop statements: %q", my)
	}
}

// mysqldump leaves routines and events out unless asked, so an export without
// them hands over a database that looks complete and has lost its stored
// procedures. A snapshot stores the same declared export, so asserting the
// export covers both; DumpFlags still feeds the CLI and MCP paths and must not
// drift from what the preset declares.
func TestDeclaredExportKeepsRoutinesAndEvents(t *testing.T) {
	my := bundledDatabasesAction(t, "mysql", "export")
	for _, flag := range DumpFlags("mysql") {
		if !strings.Contains(my, flag) {
			t.Errorf("mysql export missing %s: %q", flag, my)
		}
	}
	pg := bundledDatabasesAction(t, "postgres", "export")
	for _, flag := range DumpFlags("postgres") {
		if !strings.Contains(pg, flag) {
			t.Errorf("postgres export missing %s: %q", flag, pg)
		}
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

// A .sql.gz is how dumps travel between people. Handed the compressed bytes the
// engine client reports a few encoding errors, loads nothing and exits clean.
func TestDumpReaderUnwrapsGzip(t *testing.T) {
	const sql = "CREATE TABLE t (id int);\n"

	plain, err := DumpReader(strings.NewReader(sql))
	if err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, plain); got != sql {
		t.Errorf("plain dump = %q, want %q", got, sql)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(sql))
	_ = gz.Close()
	unwrapped, err := DumpReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, unwrapped); got != sql {
		t.Errorf("gzipped dump = %q, want %q", got, sql)
	}
}

// An empty or one-byte upload has no magic to peek at and must not be mistaken
// for a broken archive; the engine reports what it makes of it.
func TestDumpReaderPassesShortInputThrough(t *testing.T) {
	for _, in := range []string{"", "-"} {
		r, err := DumpReader(strings.NewReader(in))
		if err != nil {
			t.Fatalf("input %q: %v", in, err)
		}
		if got := readAll(t, r); got != in {
			t.Errorf("input %q came back as %q", in, got)
		}
	}
}

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// An empty gzip stream is 20 bytes and decompresses to nothing. A dump that
// produced one is a failure whatever the shell reported, so it must never be
// kept as a restorable snapshot.
func TestVerifyDumpHasContent(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.gz")
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyDumpHasContent(empty, true); err == nil {
		t.Error("an empty gzip dump passed verification")
	}

	full := filepath.Join(dir, "full.gz")
	buf.Reset()
	zw = gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte("CREATE TABLE t (id int);")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyDumpHasContent(full, true); err != nil {
		t.Errorf("a real dump failed verification: %v", err)
	}

	plain := filepath.Join(dir, "plain.sql")
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyDumpHasContent(plain, false); err == nil {
		t.Error("an empty uncompressed dump passed verification")
	}
}
