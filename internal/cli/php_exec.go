package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/agentenv"
	"github.com/geodro/lerd/internal/config"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// NewPhpCmd returns the php command — runs PHP in the appropriate FPM container.
func NewPhpCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "php [args...]",
		Short:              "Run PHP in the project's container (e.g. lerd php artisan migrate)",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE:               runPhp,
	}
}

func runPhp(_ *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	return RunPHP(cwd, args)
}

// RunPHP execs `php <args...>` inside the project's PHP-FPM container, with
// stdio wired to the current terminal. Used by `lerd php`, the vendor/bin
// fallback, and other passthrough commands that need a PHP runtime. The
// child's exit code is propagated via os.Exit; callers that need to do work
// after the child exits (e.g. sync wrappers after a failed composer remove)
// should use RunPHPCapture instead.
func RunPHP(cwd string, args []string) error {
	code, err := RunPHPCapture(cwd, args)
	if err != nil {
		return err
	}
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

// RunPHPCapture is the non-exiting variant of RunPHP. It returns the child
// process's exit code separately from any setup error (container not running,
// version detection failure, etc.), so callers can run their own work after
// the child exits before propagating the code to the parent shell.
func RunPHPCapture(cwd string, args []string) (int, error) {
	return RunPHPCaptureEnv(cwd, args, nil)
}

// phpVersionForDir resolves the PHP version a directory's commands must run on.
// The rules live in internal/php so the CLI and the MCP server cannot drift.
func phpVersionForDir(dir string) (string, error) {
	return phpDet.VersionForDir(dir)
}

// fpmContainerForDir resolves the FPM container an exec in dir should target:
// the per-site container for custom-FPM sites, otherwise the shared
// lerd-php<version>-fpm container. It resolves the site the same way version
// detection does, so a worktree beside its project reaches the parent's custom
// image rather than falling through to the shared container its vhost never uses.
func fpmContainerForDir(dir, version string) string {
	if _, parent, ok := phpDet.WorktreeRootFor(dir); ok {
		return podman.FPMContainerName(*parent, version)
	}
	if site, _ := config.FindSiteByPath(phpDet.SiteRootFor(dir)); site != nil {
		return podman.FPMContainerName(*site, version)
	}
	return "lerd-php" + strings.ReplaceAll(version, ".", "") + "-fpm"
}

// debugSiteEnvArgs returns the LERD_SITE exec flag for a CLI run in dir, so the
// debug bridge and the devtools extension tag every event with the registered
// site name. Without it the bridge falls back to the directory basename and the
// extension emits no site at all, which strands the notification (#1005). A
// worktree checkout reports its parent site, like tinker and the worktree vhost.
func debugSiteEnvArgs(dir string) []string {
	if _, parent, ok := phpDet.WorktreeRootFor(dir); ok && parent != nil && parent.Name != "" {
		return []string{"--env", "LERD_SITE=" + parent.Name}
	}
	if site, _ := config.FindSiteByPath(phpDet.SiteRootFor(dir)); site != nil && site.Name != "" {
		return []string{"--env", "LERD_SITE=" + site.Name}
	}
	return nil
}

// RunPHPCaptureEnv is RunPHPCapture with extra KEY=VALUE environment entries
// injected into the container exec — used by `lerd profile run` to set
// SPX_ENABLED so a CLI command is profiled.
func RunPHPCaptureEnv(cwd string, args []string, extraEnv []string) (int, error) {
	recordCwdActivity(cwd) // keep the site awake under idle-suspend while you work in the terminal
	// The CLI SAPI ignores a project's .user.ini, so a framework declaring
	// php.cli_ini gets it as -d on every PHP process lerd starts for it.
	args = prependPHPIniArgs(phpIniArgsForDir(cwd), args)
	version, err := phpVersionForDir(cwd)
	if err != nil {
		return 0, err
	}

	container := fpmContainerForDir(cwd, version)

	version, container, err = ensureFPMRunning(cwd, version, container)
	if err != nil {
		return 0, err
	}

	home := os.Getenv("HOME")
	composerHome := os.Getenv("COMPOSER_HOME")
	if composerHome == "" {
		// Respect XDG: prefer ~/.config/composer, fall back to ~/.composer
		xdgConfig := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfig == "" {
			xdgConfig = filepath.Join(home, ".config")
		}
		composerHome = filepath.Join(xdgConfig, "composer")
	}
	composerBin := filepath.Join(composerHome, "vendor", "bin")
	projectVendorBin := filepath.Join(cwd, "vendor", "bin")

	// A cwd the container can't reach (an ephemeral /tmp path, not parked and not
	// listed under mounts:) makes `podman exec -w <cwd>` fail with an opaque crun
	// chdir error. Refuse with a clear message instead (issue #949).
	if !podman.PathVisible(cwd, version) && !podman.PathAutoMountable(cwd) {
		return 0, fmt.Errorf("cannot run php from %s: lerd does not mount temporary system directories (/tmp, /var/tmp, /run) into the PHP container. Run from a path under your home directory or a parked directory, or add the path to mounts: in %s", cwd, config.GlobalConfigFile())
	}

	podman.EnsurePathMounted(cwd, version)
	ensureServicesForCwd(cwd)

	// PHP runs the first non-option operand as its script. When that script is an
	// absolute path the container can't read (e.g. /tmp/ide-phpinfo.php written by
	// an IDE), stream it through stdin as /dev/stdin. Only the script is eligible:
	// a later path is the script's own argument (a data file), and rewriting that
	// to /dev/stdin silently breaks is_file() checks and any script taking more
	// than one file (issue #949).
	var stdinReader io.Reader = os.Stdin
	useTTY := term.IsTerminal(int(os.Stdin.Fd()))
	if i := phpScriptArgIndex(args); i >= 0 && i < len(args) {
		arg := args[i]
		if filepath.IsAbs(arg) && !strings.HasPrefix(arg, home+"/") && arg != home && !podman.PathVisible(arg, version) {
			if data, err := os.ReadFile(arg); err == nil {
				args[i] = "/dev/stdin"
				stdinReader = bytes.NewReader(data)
				useTTY = false
			}
		}
	}

	execFlags := []string{"exec", "-i"}
	if useTTY {
		execFlags = append(execFlags, "-t")
	}

	cmdArgs := append(execFlags, "-w", cwd,
		"--env", "HOME="+home,
		"--env", "COMPOSER_HOME="+composerHome,
		"--env", "PATH="+projectVendorBin+":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:"+composerBin,
	)
	cmdArgs = append(cmdArgs, debugSiteEnvArgs(cwd)...)
	// Forward SPX_* profiler vars from the host so `SPX_ENABLED=1 php ...` (or
	// any shim'd tool like composer) reaches SPX inside the container. extraEnv
	// is applied after, so an explicit caller like `lerd profile run` wins.
	for _, e := range spxPassthroughEnv(os.Environ()) {
		cmdArgs = append(cmdArgs, "--env", e)
	}
	// Forward AI agent detection vars so agent-detector (e.g. laravel/pao)
	// still emits JSON when run inside the container.
	for _, e := range agentenv.Passthrough(os.Environ()) {
		cmdArgs = append(cmdArgs, "--env", e)
	}
	for _, e := range extraEnv {
		cmdArgs = append(cmdArgs, "--env", e)
	}
	// Point composer/git at the shared ssh-agent when it's running, so private
	// packages with passphrase-protected keys authenticate over SSH. No-op when
	// the agent isn't up (falls back to the bind-mounted on-disk keys).
	cmdArgs = append(cmdArgs, podman.SSHAuthSockEnv()...)
	cmdArgs = append(cmdArgs, container, "php")
	cmdArgs = append(cmdArgs, args...)

	cmd := podman.Cmd(cmdArgs...)
	cmd.Stdin = stdinReader
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

// phpScriptArgIndex returns the index of the script operand in a `php` argument
// list — the first token that is not an option (nor an option's separate value),
// which is the file PHP executes. Returns -1 when the invocation runs no file
// (php -r '...', php -v). Flag parsing is disabled on the php command, so this
// mirrors just enough of PHP's CLI getopt to tell a script path from a flag.
func phpScriptArgIndex(args []string) int {
	// Single-letter options that consume the following token as their value; the
	// glued forms (-dfoo=bar) carry their own value and pass through as one token.
	valueFlags := map[string]bool{
		"-c": true, "-d": true, "-r": true, "-B": true, "-R": true,
		"-F": true, "-E": true, "-S": true, "-t": true, "-z": true,
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-f" {
			return i + 1 // php -f <file>: the value is the script
		}
		if strings.HasPrefix(a, "-") {
			if valueFlags[a] {
				i++ // skip its separate value token
			}
			continue
		}
		return i
	}
	return -1
}

// spxPassthroughEnv picks the SPX_* profiler vars out of environ. When SPX is
// enabled but no report type is set, it defaults SPX_REPORT to full so the run
// lands in the Profiler view instead of a terminal flat profile.
func spxPassthroughEnv(environ []string) []string {
	var out []string
	enabled, hasReport := false, false
	for _, e := range environ {
		if !strings.HasPrefix(e, "SPX_") {
			continue
		}
		out = append(out, e)
		k, v, _ := strings.Cut(e, "=")
		switch k {
		case "SPX_ENABLED":
			enabled = v != "" && v != "0"
		case "SPX_REPORT":
			hasReport = true
		}
	}
	if enabled && !hasReport {
		out = append(out, "SPX_REPORT=full")
	}
	return out
}
