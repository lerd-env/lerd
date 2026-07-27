//go:build linux

package dns

import (
	"os"
	"path/filepath"
	"testing"
)

// WriteSudoersForUser runs as root, where the user-owned marker resolves to
// root's HOME and describes nothing about the target user. Trusting it there
// skipped the write whenever a previous root pass had left a matching marker
// behind, so an install after an uninstall came up with no drop-in at all and
// every later resolver operation prompted for a password.
func TestWriteSudoersForUserIgnoresStaleRootMarker(t *testing.T) {
	dir := t.TempDir()
	dropIn := filepath.Join(dir, "lerd")

	origPath, origMarker := lerdSudoersPath, sudoersMarkerPath
	t.Cleanup(func() { lerdSudoersPath, sudoersMarkerPath = origPath, origMarker })
	lerdSudoersPath = dropIn
	sudoersMarkerPath = func() string { return filepath.Join(dir, "sudoers.sha256") }
	stubSudoProbe(t, true, true)

	// The state that broke it: a marker claiming this exact rule is installed,
	// with no drop-in on disk.
	recordSudoersInstalled([]byte(renderLinuxSudoers("george")))

	if err := WriteSudoersForUser("george"); err != nil {
		t.Fatalf("WriteSudoersForUser: %v", err)
	}
	got, err := os.ReadFile(dropIn)
	if err != nil {
		t.Fatalf("drop-in not written despite being absent: %v", err)
	}
	if string(got) != renderLinuxSudoers("george") {
		t.Error("drop-in content does not match the rendered rule")
	}
}

// Writing is still skipped when the real file already carries the rule, so a
// package upgrade that re-runs the bootstrap does not churn /etc/sudoers.d.
func TestWriteSudoersForUserSkipsWhenFileMatches(t *testing.T) {
	dir := t.TempDir()
	dropIn := filepath.Join(dir, "lerd")

	origPath := lerdSudoersPath
	t.Cleanup(func() { lerdSudoersPath = origPath })
	lerdSudoersPath = dropIn

	content := renderLinuxSudoers("george")
	if err := os.WriteFile(dropIn, []byte(content), 0440); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dropIn)
	if err != nil {
		t.Fatal(err)
	}

	if err := WriteSudoersForUser("george"); err != nil {
		t.Fatalf("WriteSudoersForUser: %v", err)
	}
	after, err := os.Stat(dropIn)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(info.ModTime()) {
		t.Error("rewrote a drop-in that already carried the right content")
	}
}

// A rule that changed between versions must still be rewritten.
func TestWriteSudoersForUserRewritesChangedRule(t *testing.T) {
	dir := t.TempDir()
	dropIn := filepath.Join(dir, "lerd")

	origPath := lerdSudoersPath
	t.Cleanup(func() { lerdSudoersPath = origPath })
	lerdSudoersPath = dropIn

	// 0600 rather than the real 0440: the caller is root in production and
	// writes through the read-only mode, which a test user cannot do.
	if err := os.WriteFile(dropIn, []byte("an older lerd rule\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSudoersForUser("george"); err != nil {
		t.Fatalf("WriteSudoersForUser: %v", err)
	}
	got, err := os.ReadFile(dropIn)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != renderLinuxSudoers("george") {
		t.Error("stale rule was not replaced")
	}
}
