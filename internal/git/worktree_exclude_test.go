package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Nested worktree dirs sit inside the parent's working tree, so git status
// shows them as untracked. EnsureNestedWorktreeExclude writes a single
// /<base>-*/ pattern to .git/info/exclude — idempotently.
func TestEnsureNestedWorktreeExclude(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "myapp")
	if err := os.MkdirAll(filepath.Join(sitePath, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureNestedWorktreeExclude(sitePath); err != nil {
		t.Fatal(err)
	}
	if err := EnsureNestedWorktreeExclude(sitePath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(sitePath, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	want := "/myapp-*/"
	count := 0
	for _, line := range strings.Split(got, "\n") {
		if strings.TrimSpace(line) == want {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected %q exactly once in exclude, got %d times: %q", want, count, got)
	}
}

// Worktrees write .lerd.local.yaml, which has to stay out of git status even in a
// repo that does not ignore it itself.
