package serviceops

import (
	"strings"
	"testing"
)

// ValidateDatabaseName is the injection guard for every declared entity
// command: a worktree DB name derives from a git branch, and git allows quotes
// and backticks in branch names, so anything that could terminate a shell word
// or an SQL identifier's quoting must never reach substitution.
func TestValidateDatabaseName(t *testing.T) {
	// Interior dots are legal because S3 bucket names carry them (my.bucket).
	valid := []string{"acme_app", "acme_app_testing", "app2", "_private", "my-db", "A1", "crm.example.dk"}
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
		"a/b", `a\b`, "a'b", "a\"b", "a`b", "a;b", "a b",
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
