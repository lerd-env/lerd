package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestRequiredWorktreeDBChoice covers the case a shared database cannot serve:
// Magento keeps its config hash in the database, so a worktree that imports its
// own config into the parent's database breaks the parent. A definition asking
// for isolation is honoured instead of prompting.
func TestRequiredWorktreeDBChoice(t *testing.T) {
	cases := []struct {
		name string
		fw   *config.Framework
		want string
	}{
		{
			name: "no worktree block prompts as before",
			fw:   &config.Framework{},
			want: "",
		},
		{
			name: "required isolation from main clones the parent database",
			fw:   &config.Framework{Worktree: &config.FrameworkWorktree{DBIsolation: "required", DBSource: "main"}},
			want: "clone-main",
		},
		{
			name: "required isolation with no source starts empty",
			fw:   &config.Framework{Worktree: &config.FrameworkWorktree{DBIsolation: "required"}},
			want: "empty",
		},
		{
			name: "a worktree block that does not require isolation still prompts",
			fw:   &config.Framework{Worktree: &config.FrameworkWorktree{Commands: []string{"app:config:import"}}},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := requiredWorktreeDBChoice(c.fw); got != c.want {
				t.Errorf("requiredWorktreeDBChoice = %q, want %q", got, c.want)
			}
		})
	}
}

// TestWorktreeSetupArgs builds the console invocation from the definition, so a
// declared command runs as the framework's own console binary and nothing in Go
// has to know what app:config:import is.
func TestWorktreeSetupArgs(t *testing.T) {
	fw := &config.Framework{
		Console:  "bin/magento",
		Worktree: &config.FrameworkWorktree{Commands: []string{"app:config:import"}},
	}
	got := worktreeSetupArgs(fw, "app:config:import")
	want := []string{"bin/magento", "app:config:import"}
	if len(got) != len(want) {
		t.Fatalf("worktreeSetupArgs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("worktreeSetupArgs = %v, want %v", got, want)
		}
	}
}

// TestWorktreeSetupArgsNeedsAConsole guards the case where a definition declares
// commands but no console to run them with: better to skip than to shell out to
// a guessed binary.
func TestWorktreeSetupArgsNeedsAConsole(t *testing.T) {
	fw := &config.Framework{Worktree: &config.FrameworkWorktree{Commands: []string{"app:config:import"}}}
	if got := worktreeSetupArgs(fw, "app:config:import"); got != nil {
		t.Errorf("worktreeSetupArgs = %v, want nil when the framework declares no console", got)
	}
}

// An unattended setup takes the caller's database choice unless the definition
// requires isolation, since sharing would break the parent site.
func TestUnattendedWorktreeDBChoice(t *testing.T) {
	required := &config.Framework{Worktree: &config.FrameworkWorktree{DBIsolation: "required", DBSource: "main"}}
	cases := []struct {
		name      string
		fw        *config.Framework
		requested string
		want      string
	}{
		{"no framework keeps the request", nil, "empty", "empty"},
		{"nothing requested shares the parent", &config.Framework{}, "", "share"},
		{"a required isolation overrides sharing", required, "share", "clone-main"},
		{"a required isolation overrides nothing requested", required, "", "clone-main"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unattendedWorktreeDBChoice(c.fw, c.requested); got != c.want {
				t.Errorf("unattendedWorktreeDBChoice = %q, want %q", got, c.want)
			}
		})
	}
}

// An empty database is only usable once its schema is applied, and the command
// that does that is whatever the definition names, so Go never spells it.
func TestWorktreeMigrateCommand(t *testing.T) {
	fw := &config.Framework{
		Doctor:   &config.FrameworkDoctor{MigrateCommand: "migrate"},
		Commands: []config.FrameworkCommand{{Name: "migrate", Command: "php artisan migrate --force"}},
	}
	if got := worktreeMigrateCommand(fw); got != "php artisan migrate --force" {
		t.Errorf("worktreeMigrateCommand = %q, want the declared command", got)
	}
	fw.Doctor.MigrateCommand = "missing"
	if got := worktreeMigrateCommand(fw); got != "" {
		t.Errorf("a name the definition does not declare must run nothing, got %q", got)
	}
	if got := worktreeMigrateCommand(nil); got != "" {
		t.Errorf("no framework must run nothing, got %q", got)
	}
}

func writeMigrations(t *testing.T, dir string, names ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("<?php\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The database a worktree can use follows from how its schema compares with the
// parent checkout's, whose database sharing reuses and cloning copies.
func TestMigrationsDBChoice(t *testing.T) {
	cases := []struct {
		name           string
		parent, branch []string
		want           string
	}{
		{"same set shares", []string{"a", "b"}, []string{"a", "b"}, "share"},
		{"branch ahead clones and migrates", []string{"a"}, []string{"a", "b"}, "clone-main"},
		{"branch behind starts empty", []string{"a", "b"}, []string{"a"}, "empty"},
		{"diverged starts empty", []string{"a", "b"}, []string{"a", "c"}, "empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			parent, branch := filepath.Join(root, "parent"), filepath.Join(root, "branch")
			writeMigrations(t, parent, c.parent...)
			writeMigrations(t, branch, c.branch...)
			got, reason := migrationsDBChoice(parent, branch)
			if got != c.want {
				t.Errorf("choice = %q, want %q", got, c.want)
			}
			if reason == "" {
				t.Error("the choice needs a reason the caller can report")
			}
		})
	}
}

// Without a declared migrations folder, or with nothing to compare, lerd does
// not guess: the caller's choice or the parent's database stands.
func TestMigrationsDBChoiceFor_needsTheDeclaredFolder(t *testing.T) {
	root := t.TempDir()
	writeMigrations(t, filepath.Join(root, "p", "db"), "a")
	writeMigrations(t, filepath.Join(root, "w", "db"), "a", "b")
	if got, _ := migrationsDBChoiceFor(&config.Framework{}, filepath.Join(root, "p"), filepath.Join(root, "w")); got != "" {
		t.Errorf("no migrations key must not pick, got %q", got)
	}
	fw := &config.Framework{Worktree: &config.FrameworkWorktree{Migrations: "db"}}
	if got, _ := migrationsDBChoiceFor(fw, filepath.Join(root, "p"), filepath.Join(root, "w")); got != "clone-main" {
		t.Errorf("declared folder: got %q, want clone-main", got)
	}
	if got, _ := migrationsDBChoiceFor(fw, filepath.Join(root, "missing"), filepath.Join(root, "w")); got != "" {
		t.Errorf("a parent with no migrations folder must not pick, got %q", got)
	}
}

// An explicit choice and a required isolation both win over the comparison, and
// a picked copy or empty database is migrated so the branch's code can run on it.
func TestPlanUnattendedWorktreeDB(t *testing.T) {
	root := t.TempDir()
	parent, wt := filepath.Join(root, "p"), filepath.Join(root, "w")
	writeMigrations(t, filepath.Join(parent, "db"), "a")
	writeMigrations(t, filepath.Join(wt, "db"), "a", "b")
	fw := &config.Framework{Worktree: &config.FrameworkWorktree{Migrations: "db"}}

	// A SQLite worktree already has its own copy of the file, so there is no
	// schema to clone, only the branch's extra migrations to run on the copy.
	if choice, reason, migrate := planUnattendedWorktreeDB(fw, "", parent, wt, true); choice != "share" || !strings.Contains(reason, "SQLite") || !migrate {
		t.Errorf("sqlite = %q %q migrate=%v, want share on its own copy, migrated", choice, reason, migrate)
	}
	writeMigrations(t, filepath.Join(parent, "db"), "a", "b", "c")
	if choice, _, migrate := planUnattendedWorktreeDB(fw, "", parent, wt, true); choice != "share" || migrate {
		t.Errorf("sqlite behind = %q migrate=%v, want share, not migrated", choice, migrate)
	}
	_ = os.Remove(filepath.Join(parent, "db", "b"))
	_ = os.Remove(filepath.Join(parent, "db", "c"))
	if choice, reason, migrate := planUnattendedWorktreeDB(fw, "", parent, wt, false); choice != "clone-main" || reason == "" || !migrate {
		t.Errorf("picked = %q %q migrate=%v, want clone-main with a reason, migrated", choice, reason, migrate)
	}
	if choice, _, migrate := planUnattendedWorktreeDB(fw, "share", parent, wt, false); choice != "share" || migrate {
		t.Errorf("explicit share = %q migrate=%v, want share, not migrated", choice, migrate)
	}
	required := &config.Framework{Worktree: &config.FrameworkWorktree{Migrations: "db", DBIsolation: "required"}}
	if choice, _, _ := planUnattendedWorktreeDB(required, "", parent, wt, false); choice != "empty" {
		t.Errorf("required isolation = %q, want the definition's empty", choice)
	}
}
