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

func capturePasswordlessRemoval(t *testing.T, succeeds bool) *[]string {
	t.Helper()
	orig := tryPasswordlessSudoersRemoval
	t.Cleanup(func() { tryPasswordlessSudoersRemoval = orig })
	var got []string
	tryPasswordlessSudoersRemoval = func(path string) bool {
		got = append(got, path)
		return succeeds
	}
	return &got
}

// Without the marker a blind sudo would prompt, so only the non-interactive
// removal may run, and only the interactive one is kept away.
func TestRemoveSudoersGrant_NoMarkerNeverPrompts(t *testing.T) {
	withMarker(t, false)
	removed := captureRemoval(t)
	capturePasswordlessRemoval(t, false)

	if removeSudoersGrant() {
		t.Error("nothing was removed, so the removal must not report success")
	}
	if len(*removed) != 0 {
		t.Errorf("expected no prompting removal, got %v", *removed)
	}
}

// `lerd bootstrap --system`, the package maintainer path, writes the drop-in as
// root and leaves no user marker, yet the grant it writes still has to go.
func TestRemoveSudoersGrant_RemovesABootstrapGrantWithNoMarker(t *testing.T) {
	withMarker(t, false)
	captureRemoval(t)
	tried := capturePasswordlessRemoval(t, true)

	if !removeSudoersGrant() {
		t.Fatal("a drop-in written without a marker must still be removed")
	}
	if len(*tried) != 1 || (*tried)[0] != lerdSudoersPath {
		t.Fatalf("expected one passwordless removal of %s, got %v", lerdSudoersPath, *tried)
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
