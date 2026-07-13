package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSyncPHPVersionFile_WritesTheResolvedVersion covers the clamp case: the
// project asks for 8.1, the framework forces 8.5, and the file must end up saying
// what actually runs instead of leaving a stale pin for other tools to trust.
func TestSyncPHPVersionFile_WritesTheResolvedVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".php-version")
	if err := os.WriteFile(path, []byte("8.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := syncPHPVersionFile(dir, "8.5"); err != nil {
		t.Fatalf("syncPHPVersionFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "8.5\n" {
		t.Errorf(".php-version = %q, want %q", got, "8.5\n")
	}
}

// TestSyncPHPVersionFile_CreatesTheFile pins that a project with no pin gets one,
// so the version lerd resolved is visible to the repo and to other tooling.
func TestSyncPHPVersionFile_CreatesTheFile(t *testing.T) {
	dir := t.TempDir()

	if err := syncPHPVersionFile(dir, "8.4"); err != nil {
		t.Fatalf("syncPHPVersionFile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".php-version"))
	if err != nil {
		t.Fatalf("reading .php-version: %v", err)
	}
	if string(got) != "8.4\n" {
		t.Errorf(".php-version = %q, want %q", got, "8.4\n")
	}
}

// TestSyncPHPVersionFile_LeavesAMatchingFileAlone keeps a re-link from rewriting
// an unchanged file: the write would wake the watcher and trigger a pointless
// queue:restart on every link.
func TestSyncPHPVersionFile_LeavesAMatchingFileAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".php-version")
	if err := os.WriteFile(path, []byte("8.4\n"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	if err := syncPHPVersionFile(dir, "8.4"); err != nil {
		t.Fatalf("syncPHPVersionFile: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Error("syncPHPVersionFile rewrote a file that already held the resolved version")
	}
}

// TestSyncPHPVersionFile_SkipsSitesWithoutAPHPVersion guards host-proxy and custom
// container sites, which have no lerd-managed PHP version to pin.
func TestSyncPHPVersionFile_SkipsSitesWithoutAPHPVersion(t *testing.T) {
	dir := t.TempDir()

	if err := syncPHPVersionFile(dir, ""); err != nil {
		t.Fatalf("syncPHPVersionFile: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".php-version")); !os.IsNotExist(err) {
		t.Error("syncPHPVersionFile created a .php-version for a site with no PHP version")
	}
}
