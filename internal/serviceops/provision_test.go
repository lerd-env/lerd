package serviceops

import (
	"strings"
	"testing"
)

// These guard the DB identifier/literal quoting used by CreateDatabase and
// DropDatabase. A worktree DB name derives from a git branch, and git allows
// quotes and backticks in branch names, so a name reaching these sinks must not
// be able to terminate its quoting and inject SQL.
func TestEscapeIdentBacktick(t *testing.T) {
	cases := []struct{ in, want string }{
		{"acme_app", "acme_app"},
		{"a`b", "a``b"},
		{"`; DROP DATABASE x; --", "``; DROP DATABASE x; --"},
		{"plain", "plain"},
	}
	for _, tc := range cases {
		if got := escapeIdentBacktick(tc.in); got != tc.want {
			t.Errorf("escapeIdentBacktick(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEscapeIdentDQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"acme_app", "acme_app"},
		{`a"b`, `a""b`},
		{`x"; DROP DATABASE other; --`, `x""; DROP DATABASE other; --`},
	}
	for _, tc := range cases {
		if got := escapeIdentDQuote(tc.in); got != tc.want {
			t.Errorf("escapeIdentDQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEscapeSQLLiteral(t *testing.T) {
	cases := []struct{ in, want string }{
		{"acme_app", "acme_app"},
		{"a'b", "a''b"},
		{"' OR '1'='1", "'' OR ''1''=''1"},
	}
	for _, tc := range cases {
		if got := escapeSQLLiteral(tc.in); got != tc.want {
			t.Errorf("escapeSQLLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// MySQL treats a backslash as an escape character unless NO_BACKSLASH_ESCAPES is
// set, so doubling only the quote lets `\'` slip a quote through and end the
// literal. The backslash has to be doubled first.
func TestEscapeMySQLLiteral(t *testing.T) {
	cases := []struct{ in, want string }{
		{"acme_app", "acme_app"},
		{"a'b", "a''b"},
		{`a\b`, `a\\b`},
		{`\' UNION SELECT 1; -- `, `\\'' UNION SELECT 1; -- `},
	}
	for _, tc := range cases {
		if got := escapeMySQLLiteral(tc.in); got != tc.want {
			t.Errorf("escapeMySQLLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateDatabaseName(t *testing.T) {
	valid := []string{"acme_app", "acme_app_testing", "app2", "_private", "my-db", "A1"}
	for _, name := range valid {
		if err := ValidateDatabaseName(name); err != nil {
			t.Errorf("ValidateDatabaseName(%q) = %v, want nil", name, err)
		}
	}

	// Traversal segments, path separators and every SQL metacharacter must be
	// rejected before they reach filepath.Join or an interpolated statement.
	invalid := []string{
		"", "..", ".", ".hidden",
		"../../../../../../home/george/Code",
		"a/b", `a\b`, "a'b", "a\"b", "a`b", "a;b", "a b", "a.b",
		"-leading-dash", "naÏve", "app$", "app%",
	}
	for _, name := range invalid {
		if err := ValidateDatabaseName(name); err == nil {
			t.Errorf("ValidateDatabaseName(%q) = nil, want an error", name)
		}
	}

	// MySQL caps identifiers at 64 characters.
	if err := ValidateDatabaseName(strings.Repeat("a", 64)); err != nil {
		t.Errorf("64 characters should be accepted: %v", err)
	}
	if err := ValidateDatabaseName(strings.Repeat("a", 65)); err == nil {
		t.Error("65 characters should be rejected")
	}
}
