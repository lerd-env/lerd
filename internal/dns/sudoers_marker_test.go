package dns

import (
	"os"
	"path/filepath"
	"testing"
)

// stubSudoProbe pins the passwordless-grant probe so marker tests are isolated
// from the host's real sudo configuration. Inconclusive by default so the marker
// verdict stands.
func stubSudoProbe(t *testing.T, permitted, conclusive bool) {
	t.Helper()
	orig := sudoProbe
	t.Cleanup(func() { sudoProbe = orig })
	sudoProbe = func() (bool, bool) { return permitted, conclusive }
}

// The sudoers drop-in lives in /etc/sudoers.d (root-only 0750), so the invoking
// user cannot read it back to compare. InstallSudoers relies on a user-owned
// marker instead; without it the drop-in was rewritten, prompting for sudo, on
// every install.
func TestSudoersMarker(t *testing.T) {
	orig := sudoersMarkerPath
	t.Cleanup(func() { sudoersMarkerPath = orig })
	dir := t.TempDir()
	sudoersMarkerPath = func() string { return filepath.Join(dir, "sub", "sudoers.sha256") }
	stubSudoProbe(t, false, false)

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
	stubSudoProbe(t, false, false)

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
	stubSudoProbe(t, false, false)

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

// The content marker can't tell a deleted drop-in from an intact one. When the
// passwordless probe conclusively reports the grant is gone, sudoersInstalled
// must report not-installed so an ordinary install/update restores it, without
// needing dns:repair. An inconclusive or affirmative probe leaves a working
// setup alone.
func TestSudoersInstalled_ProbeInvalidatesDeletedDropIn(t *testing.T) {
	orig := sudoersMarkerPath
	t.Cleanup(func() { sudoersMarkerPath = orig })
	dir := t.TempDir()
	sudoersMarkerPath = func() string { return filepath.Join(dir, "sudoers.sha256") }

	content := []byte("user ALL=(root) NOPASSWD: /usr/bin/resolvectl\n")
	recordSudoersInstalled(content)

	// Grant conclusively gone: force a reinstall despite the matching marker.
	stubSudoProbe(t, false, true)
	if sudoersInstalled(content) {
		t.Fatal("a conclusive gone-grant probe must force reinstall (report not installed)")
	}

	// Grant live: the marker's verdict stands.
	stubSudoProbe(t, true, true)
	if !sudoersInstalled(content) {
		t.Fatal("a live grant with a matching marker should report installed")
	}

	// Inconclusive (no sudo, unsupported flag): never regress marker-only behaviour.
	stubSudoProbe(t, false, false)
	if !sudoersInstalled(content) {
		t.Fatal("an inconclusive probe must defer to the marker (report installed)")
	}
}
