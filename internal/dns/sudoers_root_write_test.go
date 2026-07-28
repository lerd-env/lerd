//go:build linux

package dns

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// The name reaching the drop-in is the first field of every rule in a file sudo
// parses, so a value that is not a user name must be refused before it lands
// rather than after, when it can no longer be read back.
func TestWriteSudoersForUserRefusesAnInvalidName(t *testing.T) {
	dir := t.TempDir()
	dropIn := filepath.Join(dir, "lerd")
	origPath := lerdSudoersPath
	t.Cleanup(func() { lerdSudoersPath = origPath })
	lerdSudoersPath = dropIn

	for _, name := range []string{
		"weird name",
		"george\nevil ALL=(ALL) NOPASSWD: ALL",
		"george ALL=(ALL) NOPASSWD: ALL #",
		"",
		"-leading-dash",
		strings.Repeat("a", 40),
	} {
		if err := WriteSudoersForUser(name); err == nil {
			t.Errorf("accepted the name %q", name)
		}
		if _, err := os.Stat(dropIn); err == nil {
			t.Fatalf("a drop-in was written for the name %q", name)
			_ = os.Remove(dropIn)
		}
	}
}

// A drop-in sudo cannot parse is worse than no grant at all, so the rendered
// content is checked before it is installed.
func TestWriteSudoersForUserRefusesContentVisudoRejects(t *testing.T) {
	dir := t.TempDir()
	dropIn := filepath.Join(dir, "lerd")
	origPath, origCheck := lerdSudoersPath, visudoCheck
	t.Cleanup(func() { lerdSudoersPath, visudoCheck = origPath, origCheck })
	lerdSudoersPath = dropIn
	visudoCheck = func(string) ([]byte, error) {
		return []byte("syntax error near line 1"), errors.New("exit status 1")
	}

	if err := WriteSudoersForUser("george"); err == nil {
		t.Error("installed a drop-in visudo rejected")
	}
	if _, err := os.Stat(dropIn); err == nil {
		t.Error("the rejected drop-in was written anyway")
	}
}

// A host with no visudo still gets its grant: the rule shape is constrained
// already, and refusing here would break the install on a minimal image.
func TestWriteSudoersForUserWritesWhenVisudoIsAbsent(t *testing.T) {
	dir := t.TempDir()
	dropIn := filepath.Join(dir, "lerd")
	origPath, origCheck, origMarker := lerdSudoersPath, visudoCheck, sudoersMarkerPath
	t.Cleanup(func() { lerdSudoersPath, visudoCheck, sudoersMarkerPath = origPath, origCheck, origMarker })
	lerdSudoersPath = dropIn
	sudoersMarkerPath = func() string { return filepath.Join(dir, "sudoers.sha256") }
	visudoCheck = func(string) ([]byte, error) { return nil, exec.ErrNotFound }

	if err := WriteSudoersForUser("george"); err != nil {
		t.Fatalf("WriteSudoersForUser: %v", err)
	}
	if _, err := os.Stat(dropIn); err != nil {
		t.Errorf("no drop-in written: %v", err)
	}
}

// The rendered rule for a name that passes validation must parse, checked
// against the real parser where the host has one.
func TestRenderedSudoersParses(t *testing.T) {
	if _, err := exec.LookPath("visudo"); err != nil {
		t.Skip("visudo not available")
	}
	if err := checkSudoersSyntax(renderLinuxSudoers("george")); err != nil {
		t.Errorf("the rule lerd renders does not parse: %v", err)
	}
}
