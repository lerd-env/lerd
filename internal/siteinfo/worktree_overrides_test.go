package siteinfo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// makeWorktreeFixture creates a fake git main-repo on disk with one worktree
// branch pointing at a separate checkout dir. Returns the main-repo path and
// the worktree checkout path so tests can drop a .lerd.yaml into either.
func makeWorktreeFixture(t *testing.T, branch string) (string, string) {
	t.Helper()
	main := t.TempDir()
	if err := os.Mkdir(filepath.Join(main, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	wtMeta := filepath.Join(main, ".git", "worktrees", branch)
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(t.TempDir(), branch+"-checkout")
	if err := os.Mkdir(checkout, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return main, checkout
}

// Worktrees with no .lerd.yaml inherit the parent site's PHP/Node versions and
// have no override flags set.
func TestEnrichGit_worktreeInheritsParentVersions(t *testing.T) {
	main, _ := makeWorktreeFixture(t, "feat-a")

	e := &EnrichedSite{Path: main, Domains: []string{"acme.test"}, PHPVersion: "8.3", NodeVersion: "22"}
	e.enrichGit()

	if len(e.Worktrees) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(e.Worktrees))
	}
	w := e.Worktrees[0]
	if w.PHPVersion != "8.3" {
		t.Errorf("PHPVersion = %q, want %q (inherited)", w.PHPVersion, "8.3")
	}
	if w.NodeVersion != "22" {
		t.Errorf("NodeVersion = %q, want %q (inherited)", w.NodeVersion, "22")
	}
	if w.PHPVersionOverride {
		t.Errorf("PHPVersionOverride = true, want false when no .lerd.yaml override")
	}
	if w.NodeVersionOverride {
		t.Errorf("NodeVersionOverride = true, want false when no .lerd.yaml override")
	}
}

// A worktree's own .lerd.yaml takes precedence and the override flag is set.
func TestEnrichGit_worktreeOverridesFromLerdYaml(t *testing.T) {
	main, checkout := makeWorktreeFixture(t, "feat-a")

	cfg := &config.ProjectConfig{PHPVersion: "8.4", NodeVersion: "24"}
	if err := config.SaveProjectConfig(checkout, cfg); err != nil {
		t.Fatalf("save .lerd.yaml: %v", err)
	}

	e := &EnrichedSite{Path: main, Domains: []string{"acme.test"}, PHPVersion: "8.3", NodeVersion: "22"}
	e.enrichGit()

	if len(e.Worktrees) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(e.Worktrees))
	}
	w := e.Worktrees[0]
	if w.PHPVersion != "8.4" {
		t.Errorf("PHPVersion = %q, want %q (from worktree's .lerd.yaml)", w.PHPVersion, "8.4")
	}
	if !w.PHPVersionOverride {
		t.Errorf("PHPVersionOverride = false, want true when worktree has its own value")
	}
	if w.NodeVersion != "24" {
		t.Errorf("NodeVersion = %q, want %q", w.NodeVersion, "24")
	}
	if !w.NodeVersionOverride {
		t.Errorf("NodeVersionOverride = false, want true")
	}
}

// A worktree's framework version is detected from its own checkout, so a
// branch that bumped Laravel in composer.json shows that version, not the
// parent's.
func TestEnrichGit_worktreeFrameworkVersion(t *testing.T) {
	main, checkout := makeWorktreeFixture(t, "feat-l13")

	composerJSON := []byte(`{
  "require": {
    "laravel/framework": "^13.0"
  }
}
`)
	if err := os.WriteFile(filepath.Join(checkout, "composer.json"), composerJSON, 0644); err != nil {
		t.Fatal(err)
	}

	e := &EnrichedSite{
		Path:          main,
		Domains:       []string{"acme.test"},
		PHPVersion:    "8.3",
		FrameworkName: "laravel",
	}
	e.enrichGit()

	if len(e.Worktrees) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(e.Worktrees))
	}
	w := e.Worktrees[0]
	// FrameworkVersion comes from a versioned framework yaml in the store
	// (e.g. ~/.local/share/lerd/frameworks/laravel@13.yaml). CI runs with
	// an empty store, so we only assert that the per-worktree detection
	// populates the label — exercising the GetFrameworkForDir(wt.Path)
	// codepath and falling back to the builtin Laravel definition.
	if w.FrameworkLabel == "" {
		t.Errorf("FrameworkLabel = empty, want at least the builtin label")
	}
}

// A worktree .lerd.yaml that only overrides PHP leaves Node inherited.
func TestEnrichGit_worktreePartialOverride(t *testing.T) {
	main, checkout := makeWorktreeFixture(t, "feat-a")

	cfg := &config.ProjectConfig{PHPVersion: "8.4"}
	if err := config.SaveProjectConfig(checkout, cfg); err != nil {
		t.Fatal(err)
	}

	e := &EnrichedSite{Path: main, Domains: []string{"acme.test"}, PHPVersion: "8.3", NodeVersion: "22"}
	e.enrichGit()

	w := e.Worktrees[0]
	if !w.PHPVersionOverride || w.PHPVersion != "8.4" {
		t.Errorf("PHPVersion = %q override=%v, want 8.4 override=true", w.PHPVersion, w.PHPVersionOverride)
	}
	if w.NodeVersionOverride {
		t.Errorf("NodeVersionOverride = true, want false when not set in .lerd.yaml")
	}
	if w.NodeVersion != "22" {
		t.Errorf("NodeVersion = %q, want %q (inherited)", w.NodeVersion, "22")
	}
}

// A worktree's own database decides it, so a plugin tab can follow the
// checkout shown rather than the main one.
func TestEnrichGit_worktreeHasItsOwnDatabase(t *testing.T) {
	main, checkout := makeWorktreeFixture(t, "feat-db")
	if err := os.WriteFile(filepath.Join(checkout, ".lerd.yaml"), []byte("db:\n  service: postgres\n  database: feat\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e := &EnrichedSite{Path: main, Domains: []string{"acme.test"}}
	e.enrichGit()
	if len(e.Worktrees) != 1 || !e.Worktrees[0].HasDatabase {
		t.Fatalf("worktrees = %+v, want feat-db with its database", e.Worktrees)
	}

	main, _ = makeWorktreeFixture(t, "feat-none")
	e = &EnrichedSite{Path: main, Domains: []string{"acme.test"}}
	e.enrichGit()
	if len(e.Worktrees) != 1 || e.Worktrees[0].HasDatabase {
		t.Fatalf("worktrees = %+v, want feat-none without a database", e.Worktrees)
	}
}
