package tui

import (
	"strings"
	"testing"
)

// A shell opened from the TUI runs in the same terminal as the TUI itself, so
// it gets the same colour capability the host tools see (#1276).
func TestShellExecArgs(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("TERM", "wezterm")
	t.Setenv("COLORTERM", "truecolor")

	got := strings.Join(shellExecArgs("lerd-php84-fpm", "/proj"), " ")
	for _, want := range []string{"exec -it", "--env=COLORTERM=truecolor", "--env=TERM=xterm-256color", "-w /proj"} {
		if !strings.Contains(got, want) {
			t.Errorf("shellExecArgs() = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "TERM=wezterm") {
		t.Errorf("shellExecArgs() = %q, the image has no terminfo for the host TERM", got)
	}

	if got := strings.Join(shellExecArgs("lerd-mysql", ""), " "); strings.Contains(got, "-w") {
		t.Errorf("shellExecArgs() = %q, want no working directory for a service container", got)
	}
}
