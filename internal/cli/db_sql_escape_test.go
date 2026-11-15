package cli

import "testing"

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
