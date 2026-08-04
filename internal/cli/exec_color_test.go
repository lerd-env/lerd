package cli

import (
	"strings"
	"testing"
)

// truecolorHost points the process environment at a terminal that can do more
// than sixteen colours, the case podman's pinned TERM=xterm otherwise hides
// from everything running in the container (#1276).
func truecolorHost(t *testing.T) {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("TERM", "foot")
	t.Setenv("COLORTERM", "truecolor")
}

func TestConsoleCmdArgs(t *testing.T) {
	truecolorHost(t)
	got := consoleCmdArgs("/proj", "lerd-php84-fpm", "artisan", true, []string{"migrate"})
	want := "exec -i -t --env=COLORTERM=truecolor --env=TERM=xterm-256color " +
		"-w /proj lerd-php84-fpm php artisan migrate"
	if strings.Join(got, " ") != want {
		t.Errorf("consoleCmdArgs() = %v, want %v", got, strings.Split(want, " "))
	}

	if got := consoleCmdArgs("/proj", "lerd-php84-fpm", "spark", false, nil); strings.Contains(strings.Join(got, " "), " -t ") {
		t.Errorf("consoleCmdArgs() = %v, want no pty without a terminal", got)
	}

	t.Setenv("NO_COLOR", "1")
	got = consoleCmdArgs("/proj", "lerd-php84-fpm", "artisan", true, nil)
	if line := strings.Join(got, " "); !strings.Contains(line, "--env=NO_COLOR=1") || strings.Contains(line, "COLORTERM") {
		t.Errorf("consoleCmdArgs() = %q, want the opt-out forwarded on its own", line)
	}
}

func TestPhpShellExecArgs(t *testing.T) {
	truecolorHost(t)
	got := strings.Join(phpShellExecArgs("lerd-php84-fpm", "/proj"), " ")
	if !strings.HasPrefix(got, "exec -it ") {
		t.Errorf("phpShellExecArgs() = %q, want an interactive exec", got)
	}
	if !strings.Contains(got, "--env=COLORTERM=truecolor") {
		t.Errorf("phpShellExecArgs() = %q, missing the terminal colour environment", got)
	}
	if !strings.Contains(got, "/root/.bun/bin") {
		t.Errorf("phpShellExecArgs() = %q, missing the bun PATH export", got)
	}
}
