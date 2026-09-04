package git

import (
	"os"
	"path/filepath"
	"testing"
)

// worktree_include carries untracked paths from the main repo into a fresh
// worktree, which git never does on its own.
func TestCopyWorktreeIncludes_copiesFileAndDirectory(t *testing.T) {
	main := t.TempDir()
	wt := t.TempDir()

	writeFile(t, filepath.Join(main, "auth.json"), "{\"token\":\"x\"}")
	writeFile(t, filepath.Join(main, "storage", "oauth-private.key"), "PRIVATE")
	writeProjectConfig(t, main, "worktree_include:\n  - auth.json\n  - storage/\n")

	copyWorktreeIncludes(main, wt)

	if got := readFile(t, filepath.Join(wt, "auth.json")); got != "{\"token\":\"x\"}" {
		t.Errorf("auth.json not copied, got %q", got)
	}
	if got := readFile(t, filepath.Join(wt, "storage", "oauth-private.key")); got != "PRIVATE" {
		t.Errorf("storage/ not copied, got %q", got)
	}
}

// A nested include creates its parent directories in the worktree.
func TestCopyWorktreeIncludes_createsParentDirs(t *testing.T) {
	main := t.TempDir()
	wt := t.TempDir()

	writeFile(t, filepath.Join(main, "config", "local", "keys.php"), "<?php")
	writeProjectConfig(t, main, "worktree_include:\n  - config/local/keys.php\n")

	copyWorktreeIncludes(main, wt)

	if got := readFile(t, filepath.Join(wt, "config", "local", "keys.php")); got != "<?php" {
		t.Errorf("nested include not copied, got %q", got)
	}
}

// The worktree's own copy wins; a re-sync must never overwrite it.
func TestCopyWorktreeIncludes_keepsExistingWorktreeCopy(t *testing.T) {
	main := t.TempDir()
	wt := t.TempDir()

	writeFile(t, filepath.Join(main, "auth.json"), "main")
	writeFile(t, filepath.Join(wt, "auth.json"), "mine")
	writeProjectConfig(t, main, "worktree_include:\n  - auth.json\n")

	copyWorktreeIncludes(main, wt)

	if got := readFile(t, filepath.Join(wt, "auth.json")); got != "mine" {
		t.Errorf("existing worktree file clobbered, got %q", got)
	}
}

// Anything that resolves outside the project root is ignored, so a committed
// .lerd.yaml can't pull files from elsewhere on the machine into a worktree.
func TestCopyWorktreeIncludes_skipsPathsOutsideProject(t *testing.T) {
	main := t.TempDir()
	wt := t.TempDir()
	outside := t.TempDir()

	writeFile(t, filepath.Join(outside, "secret.txt"), "nope")
	writeProjectConfig(t, main, "worktree_include:\n  - ../"+filepath.Base(outside)+"/secret.txt\n  - "+filepath.Join(outside, "secret.txt")+"\n")

	copyWorktreeIncludes(main, wt)

	entries, err := os.ReadDir(wt)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("escaping include copied something: %v", entries)
	}
}

// A listed path the main repo doesn't have is a no-op, not a failure.
func TestCopyWorktreeIncludes_ignoresMissingSource(t *testing.T) {
	main := t.TempDir()
	wt := t.TempDir()

	writeProjectConfig(t, main, "worktree_include:\n  - not-there.txt\n")

	copyWorktreeIncludes(main, wt)

	if _, err := os.Stat(filepath.Join(wt, "not-there.txt")); err == nil {
		t.Error("missing source produced a file in the worktree")
	}
}

func writeProjectConfig(t *testing.T, dir, body string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, ".lerd.yaml"), body)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
