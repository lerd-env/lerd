package cli

import (
	"strings"
	"testing"
)

// Without a terminal these prompts used to hand the reader a raw bubbletea
// error, "could not open TTY: open /dev/tty: device not configured", from a
// script, a CI run or an editor terminal. The answer has to be a stated default
// instead, and it has to be the one that destroys nothing.
func TestWorktreePromptFallbackWithoutATTY(t *testing.T) {
	msg, prompt := worktreeDBPromptPlan(false, "demo_feat_x", "mysql")
	if prompt {
		t.Error("no terminal means no prompt")
	}
	if !strings.Contains(msg, "demo_feat_x") {
		t.Errorf("the message should name the database, got %q", msg)
	}
	if strings.Contains(strings.ToLower(msg), "tty") || strings.Contains(msg, "bubbletea") {
		t.Errorf("the message must not leak the library failure, got %q", msg)
	}

	if _, prompt := worktreeDBPromptPlan(true, "demo_feat_x", "mysql"); !prompt {
		t.Error("a terminal must still get the prompt")
	}
}
