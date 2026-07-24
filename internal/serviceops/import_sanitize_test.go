package serviceops

import (
	"io"
	"strings"
	"testing"
)

func sanitized(t *testing.T, family, dump string) (string, []ImportIssue) {
	t.Helper()
	r, skipped := SanitizeDump(family, strings.NewReader(dump))
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), skipped()
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
