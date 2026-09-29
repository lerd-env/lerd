package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A tool that creates worktrees with plain git calls setup straight after, so
// a path lerd will never provision has to fail at once with the distinct error.
func TestSetupManagedWorktree_rejectsUnmanagedPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	err := SetupManagedWorktree(t.TempDir(), "skip", "", 30*time.Second, io.Discard)
	if !errors.Is(err, ErrNotLerdWorktree) {
		t.Fatalf("err = %v, want ErrNotLerdWorktree", err)
	}
}

// Building or migrating against a tree whose install is still running is what
// the wait exists to prevent, so a timed-out wait must stop the setup.
func TestSetupManagedWorktree_stopsWhenTheInstallNeverSettles(t *testing.T) {
	wt := registerWorktreeSite(t, "feature")
	// No vhost and no .env: the pipeline never finishes.
	var out strings.Builder
	err := SetupManagedWorktree(wt, "skip", "", time.Second, &out)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want the wait's timeout", err)
	}
	if out.Len() != 0 {
		t.Errorf("setup ran after the wait timed out: %q", out.String())
	}
}

func TestSetupManagedWorktree_finishesASettledWorktreeByPath(t *testing.T) {
	wt := registerWorktreeSite(t, "feature")
	writeWorktreeVhost(t, "feature")
	if err := os.WriteFile(filepath.Join(wt, ".env"), []byte("APP_ENV=local\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := SetupManagedWorktree(wt, "skip", "", 10*time.Second, io.Discard); err != nil {
		t.Errorf("SetupManagedWorktree: %v", err)
	}
}
