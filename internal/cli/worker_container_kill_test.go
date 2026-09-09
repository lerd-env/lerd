package cli

import (
	"strings"
	"testing"
)

// Two sites share a per-version container, so the sweep has to match the
// working directory as well as the command: a second Winter project running the
// same watcher would otherwise have its dev server killed from under it.
func TestContainerKillScript_MatchesCommandAndPath(t *testing.T) {
	script := containerKillScript("php artisan vite:watch theme-vampire", "/home/george/Lerd/winter")

	if !strings.Contains(script, `'php artisan vite:watch theme-vampire'*`) {
		t.Errorf("script does not match on the command\n%s", script)
	}
	if !strings.Contains(script, `= '/home/george/Lerd/winter' ]`) {
		t.Errorf("script does not match on the working directory\n%s", script)
	}
	// The tool a console command starts is a child of it and is the process
	// holding the port, so the group is signalled rather than the process.
	if !strings.Contains(script, `kill -TERM "-$g"`) {
		t.Errorf("script kills the process rather than its group\n%s", script)
	}
}

// A path or command carrying a quote must not be able to end the string it is
// embedded in and run something else inside the container.
func TestContainerKillScript_QuotesItsValues(t *testing.T) {
	script := containerKillScript("sh -c 'echo hi'", "/tmp/it's here")

	if strings.Contains(script, "; rm ") || strings.Contains(script, "\n; ") {
		t.Errorf("script broke out of its quoting\n%s", script)
	}
	if !strings.Contains(script, `'sh -c echo hi'*`) {
		t.Errorf("command was not embedded safely\n%s", script)
	}
	if !strings.Contains(script, `'/tmp/it'"'"'s here'`) {
		t.Errorf("path was not quoted safely\n%s", script)
	}
}

// An unregistered site has nothing to sweep, and asking podman about it would
// only be noise on a path that is already tearing a worker down.
func TestKillWorkerInContainer_UnknownSiteIsQuiet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	calls := 0
	orig := containerKillFn
	t.Cleanup(func() { containerKillFn = orig })
	containerKillFn = func(string, string) { calls++ }

	killWorkerInContainer("nosuchsite", t.TempDir(), "vite")
	if calls != 0 {
		t.Errorf("sweeps = %d, want none for a site that is not registered", calls)
	}
}

// A command carrying quotes is a process without them, so the match is made on
// both sides stripped: the unit runs php -r 'work()' and the container holds
// php -r work().
func TestContainerKillScript_MatchesAQuotedCommand(t *testing.T) {
	script := containerKillScript(`php -r 'error_log("hi"); sleep(600);'`, "/home/george/Lerd/demo2")

	if !strings.Contains(script, `'php -r error_log(hi); sleep(600);'*`) {
		t.Errorf("script did not strip the quoting from its match\n%s", script)
	}
	if !strings.Contains(script, `tr -d "\047\042"`) {
		t.Errorf("script does not strip quoting from the process it reads\n%s", script)
	}
}
