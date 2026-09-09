package cli

import (
	"strings"
	"testing"
)

// The wizard is a full-screen form, so a shell with no terminal cannot host it.
// It used to be entered anyway and died on the library's own message,
// "huh: bubbletea: error opening TTY", which says nothing about what to do.
func TestInitWithoutTerminalRefusesWithGuidance(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	orig := initInteractiveFn
	t.Cleanup(func() { initInteractiveFn = orig })
	initInteractiveFn = func() bool { return false }

	err := runInit(false)
	if err == nil {
		t.Fatal("expected lerd init to refuse without a terminal")
	}
	if strings.Contains(err.Error(), "bubbletea") || strings.Contains(err.Error(), "TTY") {
		t.Errorf("the library's own error leaked to the user: %v", err)
	}
	if !strings.Contains(err.Error(), "lerd link") {
		t.Errorf("error should point at the non-interactive way in, got: %v", err)
	}
}
