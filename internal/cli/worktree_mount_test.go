package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A FrankenPHP site's container mounts the worktrees present when its quadlet
// was written. A command in a newer sibling worktree remounts once; one that is
// already mounted, inside the project, or bound for another container does not.
func TestEnsureWorktreeMountedRemountsOnlyAMissingSiblingWorktree(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(root, "app")
	sibling := filepath.Join(root, "app-feature")
	for _, dir := range []string{filepath.Join(main, ".git", "worktrees", "feature"), sibling} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(main, ".git", "worktrees", "feature", "gitdir"), []byte(sibling+"/.git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, ".git"), []byte("gitdir: "+main+"/.git/worktrees/feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: main, PHPVersion: "8.5", Runtime: "frankenphp"}); err != nil {
		t.Fatal(err)
	}

	missing := true
	var remounts int
	origMissing, origRemount := worktreeMissingFn, remountFrankenPHPFn
	t.Cleanup(func() { worktreeMissingFn, remountFrankenPHPFn = origMissing, origRemount })
	worktreeMissingFn = func(string, string) bool { return missing }
	remountFrankenPHPFn = func(config.Site, string, string) error { remounts++; return nil }

	ensureWorktreeMounted(sibling, "8.5", "lerd-fp-app")
	if remounts != 1 {
		t.Fatalf("a sibling worktree the container lacks: remounts = %d, want 1", remounts)
	}
	ensureWorktreeMounted(sibling, "8.5", "lerd-php85-fpm")
	ensureWorktreeMounted(main, "8.5", "lerd-fp-app")
	missing = false
	ensureWorktreeMounted(sibling, "8.5", "lerd-fp-app")
	if remounts != 1 {
		t.Errorf("remounts = %d, want no more for another container, the project, or a mounted worktree", remounts)
	}
}
