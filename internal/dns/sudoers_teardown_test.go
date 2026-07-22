//go:build linux

package dns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The previous cover for this was a source-text grep over Teardown, which a
// permanently-false guard and a missing sudoers rule both passed. These exercise
// the decision instead.

func withMarker(t *testing.T, present bool) string {
	t.Helper()
	dir := t.TempDir()
	orig := sudoersMarkerPath
	t.Cleanup(func() { sudoersMarkerPath = orig })
	path := filepath.Join(dir, "sudoers.sha256")
	sudoersMarkerPath = func() string { return path }
	if present {
		if err := os.WriteFile(path, []byte("deadbeef\n"), 0644); err != nil {
			t.Fatalf("seeding marker: %v", err)
		}
	}
	return path
}

func captureRemoval(t *testing.T) *[]string {
	t.Helper()
	orig := runSudoersRemoval
	t.Cleanup(func() { runSudoersRemoval = orig })
	var got []string
	runSudoersRemoval = func(path string) { got = append(got, path) }
	return &got
}

// /etc/sudoers.d is 0750 root-only on Fedora and Arch, so the invoking user
// cannot stat the drop-in. Gating removal on that stat meant the grant was never
// removed there, which is the whole of issue #1094.
func TestRemoveSudoersGrant_DoesNotDependOnReadingTheRootOnlyPath(t *testing.T) {
	marker := withMarker(t, true)
	removed := captureRemoval(t)

	if !removeSudoersGrant() {
		t.Fatal("removal must be attempted whenever lerd recorded installing a drop-in")
	}
	if len(*removed) != 1 || (*removed)[0] != lerdSudoersPath {
		t.Fatalf("expected one removal of %s, got %v", lerdSudoersPath, *removed)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("the marker must be forgotten alongside the grant, or a later install skips rewriting it")
	}
}

// Without the marker lerd never installed a drop-in, and the grant that would
// make the removal passwordless is absent, so a blind sudo would prompt.
func TestRemoveSudoersGrant_SkippedWhenLerdNeverInstalledOne(t *testing.T) {
	withMarker(t, false)
	removed := captureRemoval(t)

	if removeSudoersGrant() {
		t.Error("removal must not be attempted when no drop-in was ever recorded")
	}
	if len(*removed) != 0 {
		t.Errorf("expected no removal, got %v", *removed)
	}
}

// The drop-in has to permit its own removal. Without this rule the teardown's
// `sudo rm` prompts for a password, which a non-interactive `uninstall --force`
// cannot answer, so the grant survives even once the removal is reached.
func TestLinuxSudoers_GrantsItsOwnRemoval(t *testing.T) {
	content := renderLinuxSudoers("tester")
	want := "tester ALL=(root) NOPASSWD: /usr/bin/rm -f " + lerdSudoersPath
	if !strings.Contains(content, want) {
		t.Fatalf("drop-in must grant its own removal.\nwant a line: %s\ngot:\n%s", want, content)
	}
}
