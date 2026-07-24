package serviceops

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// stubExtensionCreate swaps the container call out for the duration of a test.
func stubExtensionCreate(fn func(service, database, name string) error) func() {
	prev := createExtensionFn
	createExtensionFn = fn
	return func() { createExtensionFn = prev }
}

func sanitized(t *testing.T, family, dump string) (string, []ImportIssue) {
	t.Helper()
	r, notes := SanitizeDump(DumpTarget{Family: family}, strings.NewReader(dump))
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), notes().Skipped
}

// A dump from a managed provider assigns ownership to roles that only exist
// there. lerd's postgres has one role, so the statements mean nothing here and
// every one of them is an error the user cannot act on.
func TestSanitizeDumpDropsPostgresOwnershipAndPrivileges(t *testing.T) {
	dump := `CREATE TABLE public.users (id integer);
ALTER TABLE public.users OWNER TO laravel;
ALTER SEQUENCE public.users_id_seq OWNER TO laravel;
GRANT ALL ON SCHEMA public TO neon_superuser;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE cloud_admin IN SCHEMA public GRANT ALL ON TABLES TO neon_superuser;
INSERT INTO public.users VALUES (1);
`
	out, skipped := sanitized(t, "postgres", dump)
	for _, gone := range []string{"OWNER TO", "GRANT ALL", "REVOKE", "DEFAULT PRIVILEGES"} {
		if strings.Contains(out, gone) {
			t.Errorf("%s survived the filter:\n%s", gone, out)
		}
	}
	if !strings.Contains(out, "CREATE TABLE public.users") || !strings.Contains(out, "INSERT INTO public.users") {
		t.Errorf("filter ate real statements:\n%s", out)
	}
	if total(skipped) != 5 {
		t.Errorf("skipped = %+v, want 5 statements", skipped)
	}
}

// COPY data runs until a lone backslash-dot. A row that happens to start with a
// keyword is data, not a statement, and mangling it corrupts the table.
func TestSanitizeDumpLeavesCopyDataAlone(t *testing.T) {
	dump := "COPY public.notes (id, body) FROM stdin;\n" +
		"1\tGRANT ALL ON SCHEMA public TO admin\n" +
		"2\tALTER TABLE x OWNER TO bob\n" +
		"\\.\n" +
		"ALTER TABLE public.notes OWNER TO laravel;\n"
	out, skipped := sanitized(t, "postgres", dump)
	if !strings.Contains(out, "1\tGRANT ALL ON SCHEMA public TO admin") {
		t.Errorf("COPY data was filtered:\n%s", out)
	}
	if !strings.Contains(out, "2\tALTER TABLE x OWNER TO bob") {
		t.Errorf("COPY data was filtered:\n%s", out)
	}
	if strings.Contains(out, "OWNER TO laravel") {
		t.Errorf("the statement after the COPY block survived:\n%s", out)
	}
	if total(skipped) != 1 {
		t.Errorf("skipped = %+v, want 1", skipped)
	}
}

// mysql wears the same problem as a DEFINER clause, and it fails later rather
// than at import: the object is created and only breaks when it is used.
func TestSanitizeDumpStripsMysqlDefiner(t *testing.T) {
	dump := "/*!50001 CREATE ALGORITHM=UNDEFINED */\n" +
		"/*!50013 DEFINER=`laravel`@`%` SQL SECURITY DEFINER */\n" +
		"/*!50001 VIEW `v` AS SELECT 1 */;\n" +
		"CREATE DEFINER=`laravel`@`%` PROCEDURE `p`() SELECT 1;\n" +
		"INSERT INTO logs VALUES ('DEFINER=`laravel`@`%` stayed in the data');\n"
	out, skipped := sanitized(t, "mysql", dump)
	if strings.Contains(out, "DEFINER=`laravel`") && !strings.Contains(out, "stayed in the data") {
		t.Errorf("definer survived on a DDL line:\n%s", out)
	}
	if !strings.Contains(out, "CREATE  PROCEDURE `p`()") && !strings.Contains(out, "CREATE PROCEDURE `p`()") {
		t.Errorf("procedure lost its CREATE:\n%s", out)
	}
	if !strings.Contains(out, "stayed in the data") {
		t.Errorf("filter touched row data:\n%s", out)
	}
	if !strings.Contains(out, "SQL SECURITY DEFINER") {
		t.Errorf("SQL SECURITY DEFINER is not an owner and must stay:\n%s", out)
	}
	if total(skipped) != 2 {
		t.Errorf("skipped = %+v, want 2", skipped)
	}
}

// A dump that names nobody must come through byte for byte, and report nothing.
func TestSanitizeDumpLeavesACleanDumpUntouched(t *testing.T) {
	dump := "CREATE TABLE t (id int);\nINSERT INTO t VALUES (1);\n"
	for _, family := range []string{"postgres", "mysql", "mariadb"} {
		out, skipped := sanitized(t, family, dump)
		if out != dump {
			t.Errorf("%s rewrote a clean dump:\n%q", family, out)
		}
		if len(skipped) != 0 {
			t.Errorf("%s reported %+v on a clean dump", family, skipped)
		}
	}
}

// A dump whose last line has no newline must not lose it.
func TestSanitizeDumpKeepsAnUnterminatedLastLine(t *testing.T) {
	out, _ := sanitized(t, "postgres", "SELECT 1;\nSELECT 2;")
	if out != "SELECT 1;\nSELECT 2;" {
		t.Errorf("last line changed: %q", out)
	}
}

func total(issues []ImportIssue) int {
	n := 0
	for _, i := range issues {
		n += i.Count
	}
	return n
}

// A dump reaches for an extension by naming one of its types, and the statement
// that needs it must not be forwarded until the extension exists, or it fails
// exactly as it does today.
func TestSanitizeDumpCreatesADeclaredExtensionBeforeTheLineNeedingIt(t *testing.T) {
	var created []string
	var orderOK bool
	seen := ""
	restore := stubExtensionCreate(func(service, database, name string) error {
		created = append(created, service+"/"+database+"/"+name)
		orderOK = !strings.Contains(seen, "vector(1536)")
		return nil
	})
	defer restore()

	dump := "CREATE TABLE public.embeddings (\n    id bigint NOT NULL,\n    embedding public.vector(1536)\n);\n"
	target := DumpTarget{Service: "postgres-pgvector", Family: "postgres", Database: "shop",
		Extensions: []Extension{{Name: "vector", Types: []string{"vector"}}}}
	r, notes := SanitizeDump(target, strings.NewReader(dump))
	buf := make([]byte, 1)
	out := ""
	for {
		n, err := r.Read(buf)
		out += string(buf[:n])
		seen = out
		if err != nil {
			break
		}
	}
	if out != dump {
		t.Errorf("dump was rewritten:\n%q", out)
	}
	if len(created) != 1 || created[0] != "postgres-pgvector/shop/vector" {
		t.Fatalf("created = %v", created)
	}
	if !orderOK {
		t.Error("the extension was created after the line that needs it went out")
	}
	if n := notes(); len(n.Created) != 1 || n.Created[0].Count != 1 {
		t.Errorf("notes = %+v", n)
	}
}

// Two tables of the same type must not each pay for a container exec.
func TestSanitizeDumpCreatesEachExtensionOnce(t *testing.T) {
	calls := 0
	defer stubExtensionCreate(func(_, _, _ string) error { calls++; return nil })()
	dump := "CREATE TABLE a (\n    e public.vector(3)\n);\nCREATE TABLE b (\n    e public.vector(3)\n);\n"
	target := DumpTarget{Service: "pg", Family: "postgres", Database: "shop",
		Extensions: []Extension{{Name: "vector", Types: []string{"vector"}}}}
	r, _ := SanitizeDump(target, strings.NewReader(dump))
	_, _ = io.ReadAll(r)
	if calls != 1 {
		t.Errorf("created the extension %d times", calls)
	}
}

// An extension the image does not ship has to be named in the report, since the
// type error the engine raises afterwards cannot be mapped back to it.
func TestSanitizeDumpReportsAnExtensionItCouldNotCreate(t *testing.T) {
	defer stubExtensionCreate(func(_, _, _ string) error { return errors.New("not available") })()
	target := DumpTarget{Service: "pg", Family: "postgres", Database: "shop",
		Extensions: []Extension{{Name: "postgis", Types: []string{"geometry"}}}}
	r, notes := SanitizeDump(target, strings.NewReader("CREATE TABLE t (\n    shape public.geometry(Point)\n);\n"))
	_, _ = io.ReadAll(r)
	n := notes()
	if len(n.Created) != 1 || !strings.Contains(n.Created[0].Message, "postgis") {
		t.Fatalf("notes = %+v", n)
	}
	if !strings.Contains(n.Created[0].Message, "could not") {
		t.Errorf("a failure must not read as a success: %q", n.Created[0].Message)
	}
}

// The mysql families have no extensions, and a dump into one must never trigger
// a lookup for them.
func TestSanitizeDumpNeverCreatesOnMysql(t *testing.T) {
	calls := 0
	defer stubExtensionCreate(func(_, _, _ string) error { calls++; return nil })()
	target := DumpTarget{Service: "mysql", Family: "mysql", Database: "shop",
		Extensions: []Extension{{Name: "vector", Types: []string{"vector"}}}}
	r, _ := SanitizeDump(target, strings.NewReader("CREATE TABLE t (\n    e vector(3)\n);\n"))
	_, _ = io.ReadAll(r)
	if calls != 0 {
		t.Errorf("mysql import created %d extensions", calls)
	}
}

// Every freshly created database already has a public schema, so a dump that
// carries a bare CREATE SCHEMA public ends on one error nobody can act on. The
// conditional form leaves the same database behind either way.
func TestSanitizeDumpMakesCreateSchemaConditional(t *testing.T) {
	cases := []struct{ in, want string }{
		{"CREATE SCHEMA public;\n", "CREATE SCHEMA IF NOT EXISTS public;\n"},
		{"CREATE SCHEMA reporting AUTHORIZATION lerd;\n", "CREATE SCHEMA IF NOT EXISTS reporting AUTHORIZATION lerd;\n"},
		// Already conditional, and a lowercase spelling of it.
		{"CREATE SCHEMA IF NOT EXISTS public;\n", "CREATE SCHEMA IF NOT EXISTS public;\n"},
		{"create schema if not exists public;\n", "create schema if not exists public;\n"},
		// The element form cannot take IF NOT EXISTS, so it has to be left alone.
		{"CREATE SCHEMA hollywood CREATE TABLE films (title text);\n", "CREATE SCHEMA hollywood CREATE TABLE films (title text);\n"},
	}
	for _, c := range cases {
		got, _ := sanitized(t, "postgres", c.in)
		if got != c.want {
			t.Errorf("sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Only postgres has this problem, and a mysql dump must come through untouched.
func TestSanitizeDumpLeavesMysqlSchemaStatementsAlone(t *testing.T) {
	in := "CREATE SCHEMA `shop`;\n"
	if got, _ := sanitized(t, "mysql", in); got != in {
		t.Errorf("mysql dump rewritten: %q", got)
	}
}
