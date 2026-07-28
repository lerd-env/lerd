package cli

import (
	"strings"
	"testing"
)

// The name reaching these queries comes from a project's .env, and it is
// spliced into a SQL string literal. MySQL treats a backslash as an escape
// unless NO_BACKSLASH_ESCAPES is set, so the backslash has to be doubled first
// or `\'` would escape the doubled quote and end the literal. PostgreSQL keeps
// standard_conforming_strings on, where a backslash is ordinary, so doubling it
// there would corrupt the value.
func TestSQLLiteralEscaping(t *testing.T) {
	cases := []struct {
		name             string
		in, mysql, postg string
	}{
		{"plain", "shop", "shop", "shop"},
		{"quote", "sh'op", `sh''op`, `sh''op`},
		{"backslash", `sh\op`, `sh\\op`, `sh\op`},
		{"quote after backslash", `sh\'op`, `sh\\''op`, `sh\''op`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapeMySQLLiteral(tc.in); got != tc.mysql {
				t.Errorf("escapeMySQLLiteral(%q) = %q, want %q", tc.in, got, tc.mysql)
			}
			if got := escapeSQLLiteral(tc.in); got != tc.postg {
				t.Errorf("escapeSQLLiteral(%q) = %q, want %q", tc.in, got, tc.postg)
			}
		})
	}
}

// A name that closes the literal and appends its own predicate must not be able
// to change what the existence check answers. Asserted on the query the command
// actually sends, not just on the escaping helper.
func TestDatabaseExistsQueriesAreNotInjectable(t *testing.T) {
	payload := "nope' OR '1'='1"
	mysql := mysqlDatabaseExistsQuery(payload)
	if strings.Contains(mysql, "OR '1'='1'") || !strings.Contains(mysql, "''") {
		t.Errorf("mysql existence query is injectable: %s", mysql)
	}
	pg := pgDatabaseExistsQuery(payload)
	if strings.Contains(pg, "OR '1'='1'") || !strings.Contains(pg, "''") {
		t.Errorf("postgres existence query is injectable: %s", pg)
	}
	// The literal must still close exactly once, so the predicate stays one term.
	if strings.Count(mysql, "'")%2 != 0 || strings.Count(pg, "'")%2 != 0 {
		t.Errorf("a quote escaped out of its literal:\n%s\n%s", mysql, pg)
	}
}
