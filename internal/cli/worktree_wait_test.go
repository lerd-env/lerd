package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

// registerWorktreeSite wires a registered site plus one worktree checkout, the
// shape ParentSiteForWorktreeDir recognises, and returns the checkout path.
func registerWorktreeSite(t *testing.T, branch string) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	site := filepath.Join(t.TempDir(), "app")
	wt := filepath.Join(site, "wt", branch)
	makeWorktree(t, site, wt, branch)
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}
	return wt
}

// A path lerd does not manage has to be rejected outright. Callers need "not
// mine" to be distinguishable from "gave up", and burning the whole timeout on
// a path that will never be provisioned would make the command useless in a hook.
func TestWaitForManagedWorktree_rejectsUnmanagedPathWithoutWaiting(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	start := time.Now()
	err := WaitForManagedWorktree(t.TempDir(), 30*time.Second)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrNotLerdWorktree) {
		t.Fatalf("err = %v, want ErrNotLerdWorktree", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("took %s, must fail fast rather than wait out the timeout", elapsed)
	}
}

// The parent site's own path is not a worktree, so it gets the same rejection
// rather than a wait that can never be satisfied.
func TestWaitForManagedWorktree_rejectsTheParentSitePath(t *testing.T) {
	wt := registerWorktreeSite(t, "feature")
	sitePath := filepath.Dir(filepath.Dir(wt))

	if err := WaitForManagedWorktree(sitePath, 2*time.Second); !errors.Is(err, ErrNotLerdWorktree) {
		t.Errorf("err = %v, want ErrNotLerdWorktree for the parent checkout", err)
	}
}

func TestWaitForManagedWorktree_returnsOnceSettled(t *testing.T) {
	wt := registerWorktreeSite(t, "feature")
	if err := os.WriteFile(filepath.Join(wt, ".env"), []byte("APP_ENV=local\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WaitForManagedWorktree(wt, 10*time.Second); err != nil {
		t.Errorf("WaitForManagedWorktree: %v", err)
	}
}

// The regression this command exists for. node_modules/ appears with the first
// extracted package, so the artifact checks alone call a mid-flight npm ci
// finished; while an installer holds the lock the wait must not report ready.
func TestWaitForWorktreeReady_waitsOutAnInstallHoldingTheLock(t *testing.T) {
	wt := registerWorktreeSite(t, "feature")
	for name, body := range map[string]string{
		".env":                "APP_ENV=local\n",
		"composer.json":       "{}\n",
		"package.json":        "{}\n",
		"vendor/autoload.php": "<?php\n",
	} {
		p := filepath.Join(wt, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Present but mid-install, the state every content heuristic misreads.
	if err := os.MkdirAll(filepath.Join(wt, "node_modules", "lodash"), 0755); err != nil {
		t.Fatal(err)
	}

	release, err := gitpkg.LockInstall(wt, 5*time.Second)
	if err != nil {
		t.Fatalf("LockInstall: %v", err)
	}
	if err := WaitForWorktreeReady(wt, 3*time.Second); err == nil {
		release()
		t.Fatal("reported ready while an installer still held the lock")
	}

	release()
	if err := WaitForWorktreeReady(wt, 10*time.Second); err != nil {
		t.Errorf("after the install released the lock: %v", err)
	}
}

func TestWaitForWorktreeReady_timesOutWhenArtifactsNeverAppear(t *testing.T) {
	wt := registerWorktreeSite(t, "feature")

	if err := WaitForWorktreeReady(wt, 2*time.Second); err == nil {
		t.Error("want a timeout error when .env never lands")
	}
}
