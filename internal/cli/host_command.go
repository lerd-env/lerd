package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	nodeDet "github.com/geodro/lerd/internal/node"
)

// runDeclaredHostCommand runs argv on the host when the project's framework
// says these arguments cannot work in the container, and reports whether it
// took the call. The case it exists for is a desktop runtime: native:run opens
// a window, and `php` on a lerd machine is the shim into the FPM container,
// where there is no Electron and no display. The declaration names the binary,
// so no framework or package is known here by name.
//
// A declared binary that is not there yet is an error rather than a fall back
// into the container, which is the failure the declaration exists to prevent.
//
// The project-supplied host command consent gate is deliberately not consulted:
// this declaration is store data, like the vite host worker, and a runtime that
// cannot live in a container does not work at all when it is refused.
func runDeclaredHostCommand(cwd string, argv []string, extraEnv []string) (int, bool, error) {
	fw := frameworkForDir(cwd)
	hc, ok := config.MatchHostCommand(fw, argv)
	if !ok {
		return 0, false, nil
	}
	bin := hc.Binary
	if !filepath.IsAbs(bin) {
		bin = filepath.Join(cwd, hc.Binary)
	}
	if _, err := os.Stat(bin); err != nil {
		return 0, true, errors.New(hostCommandMissingMsg(argv[0], hc))
	}
	if missing := missingPHPExtensions(bin, hc.RequiresExtensions); len(missing) > 0 {
		full, err := managedHostPHP(cwd, hc.Binary, missing)
		if err != nil {
			return 0, true, err
		}
		bin = full
	}
	// The command shells out to npm, so it needs the project's Node the same way
	// a host worker does. Unmanaged Node is left to the caller's PATH, which is
	// a real login shell here rather than a unit that inherits nothing.
	cmd := exec.Command(bin, argv...)
	if lerdManagesNode() {
		cmd = nodeDet.Active().Command(hostCommandNodeVersion(cwd), bin, argv)
	}
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// A declared command can leave a server running behind it (pest's browser
	// plugin starts Playwright), which holds the inherited stdout and hangs a
	// pipeline; reap whatever outlives the run.
	grouped := groupNativeRun(cmd)
	err := cmd.Run()
	if grouped {
		reapProcessGroup(cmd)
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), true, nil
		}
		return 0, true, err
	}
	return 0, true, nil
}

// hostCommandMissingMsg reports what to do about a declared host binary the
// project has not installed yet. The command that installs it comes off the
// declaration rather than being written here, because two packages that both
// escape the container do not install the same way: nativephp/mobile installs
// through native:install-mobile precisely because the desktop package already
// owns native:install. A declaration naming none is not guessed at.
func hostCommandMissingMsg(argv0 string, hc config.HostCommand) string {
	msg := fmt.Sprintf("%s runs on the host and needs %s, which is not installed yet.", argv0, hc.Binary)
	if hc.InstallCommand == "" {
		return msg + "\nInstall the runtime it needs, then run it again."
	}
	return msg + "\nInstall the runtime first: lerd run " + hc.InstallCommand
}

// frameworkForDir resolves the framework a directory's commands run under,
// package layer merged, or nil when the directory is not a project lerd knows.
func frameworkForDir(dir string) *config.Framework {
	if name, ok := config.DetectFrameworkForDir(dir); ok {
		if fw, ok := config.GetFrameworkForDir(name, dir); ok {
			return fw
		}
	}
	return nil
}

// hostCommandNodeVersion resolves the Node a re-routed command runs under, with
// the same fallbacks a host worker uses: the project's own version, then the
// machine default. Empty is the version manager's own default alias, which is
// the right answer for a machine that has expressed no preference and keeps
// this off the platform-specific constants the worker units carry.
func hostCommandNodeVersion(cwd string) string {
	if v, err := nodeDet.DetectVersion(cwd); err == nil && v != "" {
		return v
	}
	if cfg, _ := config.LoadGlobal(); cfg != nil {
		return cfg.Node.DefaultVersion
	}
	return ""
}

// missingPHPExtensions reports which of the extensions a declaration requires
// the binary does not have, asking the binary itself rather than trusting a
// version number. A probe that cannot run reports nothing: a failed question is
// no reason to move a command into a container the declaration never asked for,
// and a binary that will not answer `php -m` fails loudly when it runs.
func missingPHPExtensions(bin string, want []string) []string {
	if len(want) == 0 {
		return nil
	}
	out, err := exec.Command(bin, "-m").Output()
	if err != nil {
		return nil
	}
	loaded := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		loaded[strings.ToLower(strings.TrimSpace(line))] = true
	}
	var missing []string
	for _, ext := range want {
		if !loaded[strings.ToLower(ext)] {
			missing = append(missing, ext)
		}
	}
	return missing
}

// managedHostPHP resolves a PHP that has what the declared binary is missing,
// downloading lerd's own pinned build the first time a project needs it. The
// runtimes projects bundle are trimmed static builds with dynamic loading
// compiled out, so an extension one lacks cannot be added to it and the only
// way to run the command is a fuller PHP.
func managedHostPHP(cwd, declaredBin string, missing []string) (string, error) {
	lacks := joinAnd(missing)
	feedback.WarnOn(os.Stderr, "%s is missing %s, so lerd runs this command with its own PHP", declaredBin, lacks)
	// The project's own PHP version, not the newest lerd pins: the command runs
	// the project's code against a composer.lock resolved for that version.
	version, err := phpVersionForDir(cwd)
	if err != nil {
		return "", err
	}
	php, err := ensureHostPHPBinary(os.Stderr, version)
	if err != nil {
		return "", fmt.Errorf("this command needs a PHP with %s and %s does not have it: %w", lacks, declaredBin, err)
	}
	// Fail here rather than inside the command: a pin that lost an extension
	// would otherwise surface as whatever the command does when it is missing,
	// which in Jump's case is a websocket bridge that dies in a log file.
	if still := missingPHPExtensions(php, missing); len(still) > 0 {
		return "", fmt.Errorf("lerd's PHP at %s is itself missing %s, so this command cannot run", php, strings.Join(still, ", "))
	}
	return php, nil
}

// joinAnd reads a short list the way the sentence around it does: "posix and
// pcntl" rather than a bare comma list.
func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
