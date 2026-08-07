package sitedoctor

import (
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Every framework spells the command that applies the schema differently, so
// the definition names it. A Symfony site was told to run migrations and given
// no way to run them, because the constant it was matched against was Laravel's
// word for it.
func TestMigrateFix_takesTheCommandTheDefinitionNames(t *testing.T) {
	symfony := &config.Framework{
		Commands: []config.FrameworkCommand{{Name: "cache:clear"}, {Name: "doctrine:migrations:migrate"}},
		Doctor:   &config.FrameworkDoctor{MigrateCommand: "doctrine:migrations:migrate"},
	}
	if got := migrateFix(symfony); got != "doctrine:migrations:migrate" {
		t.Errorf("migrateFix = %q, want the command the definition names", got)
	}
}

// A definition that names none gets no button, rather than one wired to a
// command lerd guessed at.
func TestMigrateFix_noneWithoutADeclaration(t *testing.T) {
	fw := &config.Framework{Commands: []config.FrameworkCommand{{Name: "migrate"}}}
	if got := migrateFix(fw); got != "" {
		t.Errorf("migrateFix = %q, want none when the definition declares no migrate command", got)
	}
	if got := migrateFix(nil); got != "" {
		t.Errorf("migrateFix(nil) = %q, want none", got)
	}
}

// A declaration naming a command the framework does not have would render a
// button that maps to nothing.
func TestMigrateFix_noneWhenTheNamedCommandIsMissing(t *testing.T) {
	fw := &config.Framework{
		Commands: []config.FrameworkCommand{{Name: "cache:clear"}},
		Doctor:   &config.FrameworkDoctor{MigrateCommand: "doctrine:migrations:migrate"},
	}
	if got := migrateFix(fw); got != "" {
		t.Errorf("migrateFix = %q, want none for a command the framework does not declare", got)
	}
}

// The finding a Symfony user actually sees: an empty database, with the button
// that fills it.
func TestCheckSQLiteDatabase_offersTheFrameworksOwnMigrateCommand(t *testing.T) {
	symfony := &config.Framework{
		Commands: []config.FrameworkCommand{{Name: "doctrine:migrations:migrate"}},
		Doctor:   &config.FrameworkDoctor{MigrateCommand: "doctrine:migrations:migrate"},
	}
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "DB_CONNECTION=sqlite\n")

	c, ok := checkSQLiteDatabase(dir, filepath.Join(dir, ".env"), symfony)
	if !ok || c.Status != StatusFail {
		t.Fatalf("check = %+v (ok=%v), want a failure", c, ok)
	}
	if c.Fix != "doctrine:migrations:migrate" {
		t.Errorf("fix = %q, want doctrine:migrations:migrate", c.Fix)
	}
}
