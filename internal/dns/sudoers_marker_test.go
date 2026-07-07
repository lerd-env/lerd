package dns

import (
	"os"
	"path/filepath"
	"testing"
)

// The sudoers drop-in lives in /etc/sudoers.d (root-only 0750), so the invoking
// user cannot read it back to compare. InstallSudoers relies on a user-owned
// marker instead; without it the drop-in was rewritten, prompting for sudo, on
// every install.
func TestSudoersMarker(t *testing.T) {
	orig := sudoersMarkerPath
	t.Cleanup(func() { sudoersMarkerPath = orig })
	dir := t.TempDir()
	sudoersMarkerPath = func() string { return filepath.Join(dir, "sub", "sudoers.sha256") }

	content := []byte("user ALL=(root) NOPASSWD: /usr/bin/resolvectl\n")

	if sudoersInstalled(content) {
		t.Fatal("no marker yet: should report not installed")
	}

	recordSudoersInstalled(content)

	if !sudoersInstalled(content) {
		t.Fatal("after record: same content should report installed")
	}

	if sudoersInstalled([]byte("different rule\n")) {
		t.Fatal("changed content should report not installed so it reinstalls once")
	}

	if _, err := os.ReadFile(sudoersMarkerPath()); err != nil {
		t.Fatalf("marker must be user-readable, unlike the drop-in: %v", err)
	}
}

// dns:repair must be able to restore a drop-in that was deleted out of band.
// The content marker can't detect that (the drop-in is unreadable), so repair
// forgets the marker to force InstallSudoers to rewrite. Without this the marker
// keeps reporting "installed" and repair silently no-ops on the one case it
// exists for.
func TestForgetSudoersMarker_ForcesRewrite(t *testing.T) {
	orig := sudoersMarkerPath
	t.Cleanup(func() { sudoersMarkerPath = orig })
	dir := t.TempDir()
	sudoersMarkerPath = func() string { return filepath.Join(dir, "sudoers.sha256") }

	content := []byte("user ALL=(root) NOPASSWD: /usr/bin/resolvectl\n")
	recordSudoersInstalled(content)
	if !sudoersInstalled(content) {
		t.Fatal("marker should report installed after record")
	}

	ForgetSudoersMarker()

	if sudoersInstalled(content) {
		t.Fatal("after forgetting the marker, InstallSudoers must rewrite (report not installed)")
	}
}

// A future lerd version that changes the sudoers rule must still reinstall it
// on `lerd update` (which re-runs `lerd install`), then go quiet again. This
// guards the marker against silently skipping a genuine rule change.
func TestSudoersMarker_upgradeReinstallsOnce(t *testing.T) {
	orig := sudoersMarkerPath
	t.Cleanup(func() { sudoersMarkerPath = orig })
	dir := t.TempDir()
	sudoersMarkerPath = func() string { return filepath.Join(dir, "sudoers.sha256") }

	oldRule := []byte("v1 rule\n")
	newRule := []byte("v2 rule with an extra grant\n")

	recordSudoersInstalled(oldRule)

	if sudoersInstalled(newRule) {
		t.Fatal("upgraded rule must not match the old marker, so it reinstalls")
	}
	if !sudoersInstalled(oldRule) {
		t.Fatal("old rule should still match its marker until the upgrade writes")
	}

	// The upgrade install writes the new drop-in and refreshes the marker.
	recordSudoersInstalled(newRule)

	if !sudoersInstalled(newRule) {
		t.Fatal("after upgrade write, new rule should report installed")
	}
	if sudoersInstalled(oldRule) {
		t.Fatal("old rule must no longer match after the marker is refreshed")
	}
}
