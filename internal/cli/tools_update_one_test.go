package cli

import (
	"errors"
	"strings"
	"testing"
)

// The dashboard applies one tool at a time, so a caller outside this package
// needs a per-tool entry point that reports rather than prints.
func TestUpdateOneTool(t *testing.T) {
	stubOutdatedTools(t)
	orig := updateToolFn
	t.Cleanup(func() { updateToolFn = orig })

	var asked []string
	updateToolFn = func(_ *pinnedTools, name string) error {
		asked = append(asked, name)
		if name == "composer" {
			return errors.New("checksum mismatch for composer")
		}
		return nil
	}

	if err := UpdateOneTool("mkcert"); err != nil {
		t.Errorf("UpdateOneTool(mkcert) = %v, want nil", err)
	}
	err := UpdateOneTool("composer")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("UpdateOneTool(composer) = %v, want the underlying failure", err)
	}
	if len(asked) != 2 || asked[0] != "mkcert" || asked[1] != "composer" {
		t.Errorf("updated %v, want exactly the two named tools", asked)
	}
}

// A name that is not a tool lerd manages must be refused rather than turned
// into a download path.
func TestUpdateOneToolRejectsAnUnknownName(t *testing.T) {
	stubOutdatedTools(t)
	orig := updateToolFn
	t.Cleanup(func() { updateToolFn = orig })
	called := false
	updateToolFn = func(*pinnedTools, string) error { called = true; return nil }

	for _, name := range []string{"", "curl", "../composer", "composer;rm -rf /"} {
		if err := UpdateOneTool(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if called {
		t.Error("an unknown name reached the installer")
	}
}
