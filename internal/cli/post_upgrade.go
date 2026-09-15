package cli

import (
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/version"
)

// upgradeSkipCommands never trigger the reapply: `install` is what the reapply
// runs, the others either do their own or are daemons that would be restarted
// out from under themselves.
//
// The shimmed tools are here for a different reason. Each is a file on PATH
// that execs lerd, so the caller asked for npm or php and is waiting on its
// output, frequently from a build script with no terminal of its own. Pulling
// images and restarting every container underneath one turns a compile into a
// reinstall, which is what `make build-ui` running lerd's own npm shim did.
// The reapply is not lost, only deferred to the next command someone typed.
var upgradeSkipCommands = map[string]bool{
	"install":       true,
	"bootstrap":     true,
	"post-upgrade":  true,
	"update":        true,
	"uninstall":     true,
	"serve-ui":      true,
	"watch":         true,
	"tray":          true,
	"mcp":           true,
	"dns-forwarder": true,

	// Shims written by addShellShims.
	"php":         true,
	"composer":    true,
	"node":        true,
	"npm":         true,
	"npx":         true,
	"client-exec": true,
}

// installInProgressEnv marks the process tree of a running `lerd install`.
// The version stamp is only written when the install finishes, so a helper the
// install shells out to would otherwise read the install it belongs to as a
// pending upgrade and reapply the whole environment from inside it.
const installInProgressEnv = "LERD_INSTALL_IN_PROGRESS"

// markInstallInProgress makes every process the install spawns skip the reapply.
func markInstallInProgress() {
	_ = os.Setenv(installInProgressEnv, "1")
}

// devVersion matches the `git describe` versions a build from a checkout
// carries, which change with every commit.
var devVersion = regexp.MustCompile(`-\d+-g[0-9a-f]+`)

// ApplyPendingUpgrade reapplies the environment when the binary was replaced
// since the last install. `lerd update` already does this for a self-updating
// install by re-execing `lerd install --from-update`; a package manager swaps
// the binary and runs nothing, so brew, apt and dnf installs were left with the
// second half of the install never happening (#1432).
func ApplyPendingUpgrade(cmd *cobra.Command) {
	running := version.Version
	if !shouldApplyUpgrade(topLevelCommand(cmd), running, readInstalledVersion(), isSetUp(), interactiveTerminal()) {
		return
	}

	// Record it first so a second shell doesn't start the same reapply, and
	// clear it again on failure so the next command tries once more.
	writeInstalledVersion(running)
	feedback.Begin()
	feedback.Line("lerd was upgraded to v" + running + ", applying infrastructure changes")
	if err := reexecInstallReconcile(); err != nil {
		clearInstalledVersion()
		feedback.Warn("could not finish applying the upgrade, run `lerd install`: %v", err)
	}
}

// shouldApplyUpgrade reports whether this invocation is the one to reapply the
// environment: a set-up install, running a release it has not set up yet, on a
// command at a terminal that can afford the wait.
func shouldApplyUpgrade(command, running, installed string, setUp, interactive bool) bool {
	if !isReleaseVersion(running) || !setUp || !interactive {
		return false
	}
	if os.Getenv(installInProgressEnv) != "" {
		return false
	}
	if upgradeSkipCommands[command] {
		return false
	}
	return installed != running
}

// isReleaseVersion reports whether the running binary is one a user could have
// installed from a release, as opposed to a build from a checkout.
func isReleaseVersion(v string) bool {
	if v == "" || v == "dev" || strings.Contains(v, "dirty") {
		return false
	}
	return !devVersion.MatchString(v)
}

// topLevelCommand names the command as the user typed it after `lerd`, so a
// subcommand is judged by the group it belongs to.
func topLevelCommand(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	for cmd.Parent() != nil && cmd.Parent().Parent() != nil {
		cmd = cmd.Parent()
	}
	return cmd.Name()
}

// isSetUp reports whether `lerd install` has ever run on this machine. A fresh
// package install has a binary and nothing else, and the install it still owes
// asks questions that must not be answered behind the user's back.
func isSetUp() bool {
	_, err := os.Stat(config.GlobalConfigFile())
	return err == nil
}

// interactiveTerminal reports whether there is someone to watch an install
// that restarts services and can take minutes. A script, a daemon or a shim in
// a pipe is left alone.
func interactiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func readInstalledVersion() string {
	data, err := os.ReadFile(config.InstalledVersionFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeInstalledVersion(v string) {
	path := config.InstalledVersionFile()
	if err := os.MkdirAll(config.DataDir(), 0755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(v+"\n"), 0644)
}

func clearInstalledVersion() {
	_ = os.Remove(config.InstalledVersionFile())
}
