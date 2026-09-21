package sitedoctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// macOS keys a Files and Folders grant to the binary that asked, and the fnm
// lerd downloads carries no stable signing identity, so the grant dies with the
// next pinned version and the prompt comes back for a project that never moved.
func TestProtectedPathCheck(t *testing.T) {
	home := t.TempDir()
	under := filepath.Join(home, "Documents", "GitHub", "app")
	if err := os.MkdirAll(under, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "Developer", "app")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, ok := protectedPathCheck("darwin", under, home, "fnm"); !ok {
		t.Error("a site under Documents on macOS with the bundled fnm must be flagged")
	}
	if _, ok := protectedPathCheck("darwin", outside, home, "fnm"); ok {
		t.Error("a site outside the guarded folders has nothing to warn about")
	}
	if _, ok := protectedPathCheck("linux", under, home, "fnm"); ok {
		t.Error("only macOS guards these folders; Linux must stay quiet")
	}
	// nvm runs as a shell function inside the terminal the user already granted,
	// so it never asks on its own and there is nothing to tell them. The same
	// goes for mise, which the grant survives.
	if _, ok := protectedPathCheck("darwin", under, home, "nvm"); ok {
		t.Error("nvm sidesteps the prompt, so the check must not fire for it")
	}
	if _, ok := protectedPathCheck("darwin", under, home, "mise"); ok {
		t.Error("mise keeps the grant across an update, so there is nothing to warn about")
	}

	c, _ := protectedPathCheck("darwin", under, home, "fnm")
	if c.Status != StatusWarn {
		t.Errorf("status = %q, want a warning: the site works, it just keeps asking", c.Status)
	}
	if !strings.Contains(c.Detail, "node:manager mise") {
		t.Errorf("detail = %q, want the way out named", c.Detail)
	}
}

// TCC resolves a path to its real one, so a project parked under Documents
// through a symlink elsewhere is still guarded and has to be flagged.
func TestProtectedPathCheckFollowsSymlinks(t *testing.T) {
	home := t.TempDir()
	real := filepath.Join(home, "Documents", "app")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "app-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, ok := protectedPathCheck("darwin", link, home, "fnm"); !ok {
		t.Error("a symlink into Documents points at guarded files all the same")
	}
}
