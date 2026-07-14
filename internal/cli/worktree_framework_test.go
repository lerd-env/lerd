package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestRequiredWorktreeDBChoice covers the case a shared database cannot serve:
// Magento keeps its config hash in the database, so a worktree that imports its
// own config into the parent's database breaks the parent. A definition asking
// for isolation is honoured instead of prompting.
func TestRequiredWorktreeDBChoice(t *testing.T) {
	cases := []struct {
		name string
		fw   *config.Framework
		want string
	}{
		{
			name: "no worktree block prompts as before",
			fw:   &config.Framework{},
			want: "",
		},
		{
			name: "required isolation from main clones the parent database",
			fw:   &config.Framework{Worktree: &config.FrameworkWorktree{DBIsolation: "required", DBSource: "main"}},
			want: "clone-main",
		},
		{
			name: "required isolation with no source starts empty",
			fw:   &config.Framework{Worktree: &config.FrameworkWorktree{DBIsolation: "required"}},
			want: "empty",
		},
		{
			name: "a worktree block that does not require isolation still prompts",
			fw:   &config.Framework{Worktree: &config.FrameworkWorktree{Commands: []string{"app:config:import"}}},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := requiredWorktreeDBChoice(c.fw); got != c.want {
				t.Errorf("requiredWorktreeDBChoice = %q, want %q", got, c.want)
			}
		})
	}
}

// TestWorktreeSetupArgs builds the console invocation from the definition, so a
// declared command runs as the framework's own console binary and nothing in Go
// has to know what app:config:import is.
func TestWorktreeSetupArgs(t *testing.T) {
	fw := &config.Framework{
		Console:  "bin/magento",
		Worktree: &config.FrameworkWorktree{Commands: []string{"app:config:import"}},
	}
	got := worktreeSetupArgs(fw, "app:config:import")
	want := []string{"bin/magento", "app:config:import"}
	if len(got) != len(want) {
		t.Fatalf("worktreeSetupArgs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("worktreeSetupArgs = %v, want %v", got, want)
		}
	}
}

// TestWorktreeSetupArgsNeedsAConsole guards the case where a definition declares
// commands but no console to run them with: better to skip than to shell out to
// a guessed binary.
func TestWorktreeSetupArgsNeedsAConsole(t *testing.T) {
	fw := &config.Framework{Worktree: &config.FrameworkWorktree{Commands: []string{"app:config:import"}}}
	if got := worktreeSetupArgs(fw, "app:config:import"); got != nil {
		t.Errorf("worktreeSetupArgs = %v, want nil when the framework declares no console", got)
	}
}
