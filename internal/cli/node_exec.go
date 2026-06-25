package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	nodeDet "github.com/geodro/lerd/internal/node"
	"github.com/spf13/cobra"
)

// NewNodeCmd returns the node command.
func NewNodeCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "node [args...]",
		Short:              "Run node using the project's version via fnm",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runWithFnm("node", args, true)
		},
	}
}

// NewNpmCmd returns the npm command.
func NewNpmCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "npm [args...]",
		Short:              "Run npm using the project's node version via fnm",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runWithFnm("npm", args, true)
		},
	}
}

// NewNpxCmd returns the npx command.
func NewNpxCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "npx [args...]",
		Short:              "Run npx using the project's node version via fnm",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runWithFnm("npx", args, true)
		},
	}
}

// runNpmCaptured runs `npm <args>` in dir using the project's Node version via
// fnm, capturing combined output. Unlike runWithFnm (which streams to the
// terminal and os.Exit's on failure for CLI use), this is for non-interactive
// callers like the UI: it returns the output and never exits the process, and
// it surfaces a failed `fnm install` instead of swallowing it. Shares the same
// fnm lookup, version detection, and npm_config_prefix handling as runWithFnm.
func runNpmCaptured(dir string, args ...string) (string, error) {
	fnm := filepath.Join(config.BinDir(), "fnm")
	if _, err := os.Stat(fnm); err != nil {
		return "", fmt.Errorf("fnm not found at %s, run 'lerd install' first", fnm)
	}

	version, _ := nodeDet.DetectVersion(dir)
	if version == "" {
		version = "default"
	}
	if version != "default" {
		if out, err := exec.Command(fnm, "install", version).CombinedOutput(); err != nil {
			return "", fmt.Errorf("installing Node %s via fnm: %s", version, strings.TrimSpace(string(out)))
		}
	} else if exec.Command(fnm, "exec", "--using=default", "--", "true").Run() != nil {
		return "", fmt.Errorf("no Node.js version available via lerd, run: lerd node:install 22")
	}

	cmdArgs := append([]string{"exec", "--using=" + version, "--", "npm"}, args...)
	cmd := exec.Command(fnm, cmdArgs...)
	cmd.Dir = dir
	cmd.Env = append(shimLeadingEnv(os.Environ()), "npm_config_prefix="+config.NodeGlobalDir())
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// bunRunnerFor returns the host bun binary to use for dir, or "" to fall back
// to npm/fnm. When the project is configured for bun but no bun is installed and
// warn is set it prints a one-line install hint and returns "" so the caller uses
// npm instead of failing. warn is true only for interactive CLI runs; the worker
// unit-generation path (which also runs on the daemon-side idle resume) passes
// false so the hint doesn't spam the daemon's stderr on every reconcile. lerd
// never installs or version-manages the host bun itself.
func bunRunnerFor(dir string, warn bool) string {
	// An explicit `js_runtime: node` pins the project to Node and opts out of
	// both bun detection and the no-Node fallback — for apps bun can't run, e.g.
	// NestJS with native addons. (JSRuntime normalizes node/nodejs/npm.)
	if nodeDet.JSRuntime(dir) == "node" {
		return ""
	}
	bun := nodeDet.BunPath()
	if nodeDet.UsesBun(dir) {
		if bun == "" && warn {
			fmt.Fprintln(os.Stderr, "lerd: this project uses bun but bun isn't installed — falling back to npm.")
			fmt.Fprintln(os.Stderr, "      install it with: curl -fsSL https://bun.sh/install | bash")
		}
		return bun
	}
	// Fallback: when lerd isn't managing Node and there's no system Node on
	// PATH but bun is installed, use bun as the JS runtime — it's a drop-in for
	// npm and is the only thing left that can run JS (e.g. after node:unmanage).
	if bun != "" && !lerdManagesNode() && !systemNodeAvailable() {
		return bun
	}
	return ""
}

// systemNodeAvailable reports whether a `node` binary is resolvable on PATH
// (outside lerd's own fnm shims). Used to decide the bun fallback.
func systemNodeAvailable() bool {
	return nodeDet.SystemNodeAvailable()
}

// runBun execs the host bun binary in dir, streaming to the terminal. bun is
// self-contained, so unlike node it needs no fnm wrapper or version pin. When
// exitOnFail is set it os.Exit's with the child's code to mirror runWithFnm's
// CLI behaviour; callers that fold the run behind a feedback step (setup) pass
// false so the non-zero exit is returned and the step loop can report it.
func runBun(dir, bun string, args []string, exitOnFail bool) error {
	cmd := exec.Command(bun, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = shimLeadingEnv(os.Environ())
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exitOnFail {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	return nil
}

// runJSInstall installs JS dependencies in dir: `bun install` when the project
// uses bun (frozen adds --frozen-lockfile), otherwise `npm ci`/`npm install`
// via fnm.
func runJSInstall(dir string, frozen bool) error {
	if bun := bunRunnerFor(dir, true); bun != "" {
		args := []string{"install"}
		// --frozen-lockfile only makes sense when a bun lockfile exists; npm's
		// package-lock (the `frozen` arg) doesn't apply to bun.
		for _, lf := range []string{"bun.lockb", "bun.lock"} {
			if _, err := os.Stat(filepath.Join(dir, lf)); err == nil {
				args = append(args, "--frozen-lockfile")
				break
			}
		}
		return runBun(dir, bun, args, false)
	}
	if frozen {
		return runWithFnm("npm", []string{"ci"}, false)
	}
	return runWithFnm("npm", []string{"install"}, false)
}

// runJSScript runs a package.json script in dir via `bun run <script>` when the
// project uses bun, otherwise `npm run <script>` via fnm.
func runJSScript(dir, script string) error {
	if bun := bunRunnerFor(dir, true); bun != "" {
		return runBun(dir, bun, []string{"run", script}, false)
	}
	return runWithFnm("npm", []string{"run", script}, false)
}

// shimLeadingEnv prepends lerd's bin dir (home of the `php` shim) to PATH so
// child processes (e.g. Vite's wayfinder step) resolve `php` to lerd's managed
// version instead of a global host php ahead of the shim on PATH — issue #381.
func shimLeadingEnv(env []string) []string {
	binDir := config.BinDir()
	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		if name, val, ok := strings.Cut(kv, "="); ok && strings.EqualFold(name, "PATH") {
			out = append(out, name+"="+binDir+string(os.PathListSeparator)+val)
			found = true
			continue
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, "PATH="+binDir)
	}
	return out
}

func runWithFnm(bin string, args []string, exitOnFail bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	recordCwdActivity(cwd) // keep the site awake under idle-suspend while you work in the terminal

	version, _ := nodeDet.DetectVersion(cwd)
	// Empty means the user has no .nvmrc / .node-version / global default; fall
	// through to the fnm `default` alias so we still surface a friendly error
	// instead of an unhelpful "Can't find version in dotfiles".
	if version == "" {
		version = "default"
	}

	fnm := filepath.Join(config.BinDir(), "fnm")
	if _, err := os.Stat(fnm); err != nil {
		return fmt.Errorf("fnm not found at %s — run 'lerd install' first", fnm)
	}

	if version != "default" {
		_ = exec.Command(fnm, "install", version).Run()
	} else if exec.Command(fnm, "exec", "--using=default", "--", "true").Run() != nil {
		return fmt.Errorf("no Node.js version available via lerd — run: lerd node:install 22")
	}

	cmdArgs := []string{"exec", "--using=" + version, "--", bin}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command(fnm, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	manageGlobals := bin == "npm" || bin == "npx"
	prefix := config.NodeGlobalDir()
	cmd.Env = shimLeadingEnv(os.Environ())
	if manageGlobals {
		if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err == nil {
			cmd.Env = append(cmd.Env, "npm_config_prefix="+prefix)
		}
	}
	runErr := cmd.Run()
	if manageGlobals {
		if syncErr := syncNodeGlobalBins(filepath.Join(prefix, "bin"), config.BinDir(), fnm); syncErr != nil {
			fmt.Fprintf(os.Stderr, "lerd: warning: failed to sync npm global wrappers: %v\n", syncErr)
		}
	}
	if runErr != nil {
		if exit, ok := runErr.(*exec.ExitError); ok && exitOnFail {
			os.Exit(exit.ExitCode())
		}
		return runErr
	}
	return nil
}
