package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/shims"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// clientShimPrompter returns the host-conflict prompter for shim reconciles run
// from an interactive terminal (install, service install). It asks, defaulting
// to no, before shadowing a tool the user already has. Non-interactive callers
// pass nil so the conflict is left for a later interactive run or the web UI.
func clientShimPrompter() shims.Prompter {
	if !isInteractive() {
		return nil
	}
	return func(tool string) (bool, bool) {
		e := confirmInstallPromptDefault(
			fmt.Sprintf("You already have %s installed; install lerd's %s shim (it shadows yours on PATH)?", tool, tool), false)
		return e, true
	}
}

// NewShimsCmd returns the `shims` command group for managing the client-tool
// host shims services expose (mysqldump, pg_dump, psql…). With no subcommand it
// lists the current state.
func NewShimsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shims",
		Short: "Manage the client-tool shims services expose on your PATH",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return runShimsList() },
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List client-tool shims and whether each is installed",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return runShimsList() },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "add <tool>",
		Short: "Install the host shim for a client tool (e.g. mysqldump)",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return runShimsSet(args[0], true) },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "remove <tool>",
		Short: "Remove the host shim for a client tool",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return runShimsSet(args[0], false) },
	})
	return cmd
}

func runShimsList() error {
	list := shims.List()
	if len(list) == 0 {
		fmt.Println("No installed service exposes client tools.")
		return nil
	}
	for _, s := range list {
		state := "undecided"
		if s.Decided {
			if s.Enabled {
				state = "installed"
			} else {
				state = "removed"
			}
		}
		note := ""
		if s.HostHas {
			note = "  (you have your own " + s.Tool + " on PATH)"
		}
		fmt.Printf("  %-14s %-12s %-9s%s\n", s.Tool, s.Service, state, note)
	}
	return nil
}

func runShimsSet(tool string, enabled bool) error {
	if err := shims.Set(tool, enabled); err != nil {
		return err
	}
	feedback.Begin()
	if enabled {
		feedback.Done("installed shim " + feedback.Val(tool))
	} else {
		feedback.Done("removed shim " + feedback.Val(tool))
	}
	return nil
}

// argsSpecifyHost reports whether the tool arguments already name a host, in
// which case the shim leaves the connection untouched (an external database).
// Both the postgres and mysql client families use -h / --host for the host.
func argsSpecifyHost(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--host" || strings.HasPrefix(a, "--host=") || (strings.HasPrefix(a, "-h") && len(a) > 2) {
			return true
		}
		// A connection URI (scheme://…) or a libpq conninfo string carries its own
		// host. A "host=" is only read as a conninfo key when the whole arg is
		// key=value pairs, so a host= inside a SQL body (passed via -c/-e, whether
		// spaced or inline) is never mistaken for one.
		if isConnURI(a) || (strings.Contains(a, "host=") && looksLikeConninfo(a)) {
			return true
		}
	}
	return false
}

// isConnURI reports whether a is a database connection URI, matched by a known
// scheme prefix rather than a bare "://" so a URL literal inside a query value
// is not mistaken for a connection target.
func isConnURI(a string) bool {
	for _, scheme := range []string{"postgres://", "postgresql://", "mysql://", "mariadb://"} {
		if strings.HasPrefix(a, scheme) {
			return true
		}
	}
	return false
}

// looksLikeConninfo reports whether a is a libpq conninfo string: every
// whitespace-separated token is a key=value pair. A SQL query fails this on its
// first bare word (SELECT, WHERE…), so it separates "host=db port=5432" from a
// "WHERE host='x'" predicate without knowing the tool or its flags.
func looksLikeConninfo(a string) bool {
	fields := strings.Fields(a)
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if i := strings.IndexByte(f, '='); i <= 0 {
			return false
		}
	}
	return true
}

// hostEnvSet reports whether the caller pointed the tool at a host via the
// family's environment variable, which also counts as an explicit target.
func hostEnvSet(tool string) bool {
	if isPostgresTool(tool) {
		return os.Getenv("PGHOST") != ""
	}
	return os.Getenv("MYSQL_HOST") != ""
}

// localCredsEnv returns the admin credentials for the backing lerd service so a
// hostless dump connects to the local server. Postgres needs the superuser name
// (the container user is root, which is not a role); the mysql families default
// to root already, so only the password is needed.
func localCredsEnv(tool string) []string {
	if isPostgresTool(tool) {
		return []string{"PGUSER=postgres", "PGPASSWORD=lerd"}
	}
	return []string{"MYSQL_PWD=lerd"}
}

func isPostgresTool(tool string) bool {
	return tool == "psql" || strings.HasPrefix(tool, "pg_")
}

// isSQLTool reports whether a tool has a site→service resolver (the SQL
// families read DB_HOST). Redis, valkey and mongo tools have no such mapping
// yet, so they keep the global-owner default.
func isSQLTool(tool string) bool {
	return isPostgresTool(tool) || tool == "mysql" || tool == "mysqldump"
}

// siteServiceForTool returns the lerd service the project at cwd points its
// DB_HOST at, so a bare dump run from a project targets that project's own
// database service (e.g. a mariadb-backed project routes to lerd-mariadb-<v>
// rather than the global mysql owner). Returns "" when there is no lerd DB host
// to adopt; the caller falls back to the owner, and ResolveTarget still ignores
// a service that does not actually expose the tool (so a Postgres project's
// lerd-postgres host is not adopted by mysqldump).
func siteServiceForTool(cwd, tool string) string {
	if !isSQLTool(tool) {
		return ""
	}
	host := envfile.ReadKey(filepath.Join(siteRootFor(cwd), ".env"), "DB_HOST")
	svc, ok := strings.CutPrefix(host, "lerd-")
	if !ok || svc == "" {
		return ""
	}
	return svc
}

// pathUnder reports whether path is base or nested under it, so a cwd already
// covered by the home mount isn't mounted a second time.
func pathUnder(path, base string) bool {
	return path == base || strings.HasPrefix(path, strings.TrimSuffix(base, "/")+"/")
}

// NewClientExecCmd returns the hidden `client-exec` command every client shim
// dispatches to. It resolves the service container backing the tool, starts it
// if it is down, mounts the working directory so dumps and CA certs on the host
// are reachable, and execs the tool with all arguments passed straight through.
func NewClientExecCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "client-exec <tool> [args...]",
		Short:              "Run a service client tool (mysqldump, pg_dump…) in its container",
		Hidden:             true,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runClientExec(args[0], args[1:])
		},
	}
}

func runClientExec(tool string, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// A hostless dump adopts the current project's own database service, falling
	// back to the global owner; an explicit -h (external) always uses the owner
	// and passes straight through.
	hostGiven := argsSpecifyHost(args) || hostEnvSet(tool)
	prefer := ""
	if !hostGiven {
		prefer = siteServiceForTool(cwd, tool)
	}
	target, ok := shims.ResolveTarget(tool, prefer)
	if !ok {
		return fmt.Errorf("no installed service exposes the %q client tool", tool)
	}

	// Autostart the backing service so its image is present and, for a dump
	// against a local lerd database (-h lerd-<service>), the server is up.
	if err := ensureServiceRunning(target.Service); err != nil {
		return fmt.Errorf("could not start %s: %w", target.Service, err)
	}

	image := podman.InstalledImage("lerd-" + target.Service)
	if image == "" {
		return fmt.Errorf("could not resolve the image for service %q", target.Service)
	}

	// When the caller names no host, default the connection to the resolved lerd
	// service with its admin credentials, so `pg_dump mydb` / `mysqldump mydb`
	// work against the local server out of the box. Only for the SQL tools: they
	// share the -h host flag and have known admin credentials, whereas mongosh
	// reads -h as --help and redis-cli/mongo tools have their own conventions, so
	// those pass through and expect an explicit host.
	var defaultEnv []string
	if !hostGiven && isSQLTool(tool) {
		args = append([]string{"-h", "lerd-" + target.Service}, args...)
		defaultEnv = localCredsEnv(tool)
	}

	// Resolve the first candidate binary that exists in the image, then exec it
	// with the shim's args as sh's positional parameters so no quoting is lost.
	var probe strings.Builder
	for i, b := range target.Binaries {
		if i > 0 {
			probe.WriteString(" || ")
		}
		probe.WriteString("command -v " + podman.ShellQuote(b))
	}
	shellCmd := "exec $(" + probe.String() + ") \"$@\""

	// Run the tool in a throwaway container from the service image rather than
	// exec-ing into the long-running service: nothing is mounted into or
	// restarts the persistent database container. Mount the home dir read-write
	// so the tool reads a CA cert and writes its output file (an IDE's
	// --result-file, pg_dump -f) to a host path exactly like a native client;
	// rootless podman maps container root to the host user, so the file lands
	// owned by you. A cwd outside home is mounted alongside. Joined to the lerd
	// network so a local target (-h lerd-<service>) resolves and external hosts
	// stay reachable.
	runFlags := []string{"run", "--rm", "-i", "--network", "lerd", "--entrypoint", "sh"}
	home, _ := os.UserHomeDir()
	mounted := map[string]bool{}
	addMount := func(p string) {
		if p == "" || p == "/" || mounted[p] {
			return
		}
		runFlags = append(runFlags, "-v", p+":"+p)
		mounted[p] = true
	}
	addMount(home)
	if home == "" || !pathUnder(cwd, home) {
		addMount(cwd)
	}
	runFlags = append(runFlags, "-w", cwd)
	// Allocate a pty only for a fully interactive run. If stdout is redirected
	// (mysqldump > dump.sql, pg_dump -Fc > dump.dump), a pty's line discipline
	// would translate LF to CRLF and corrupt the dump, so both stdin and stdout
	// must be terminals.
	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		runFlags = append(runFlags, "-t")
	}
	// Local-target defaults first, so the caller's own DB-client env (forwarded
	// next) always wins over them.
	for _, e := range defaultEnv {
		runFlags = append(runFlags, "-e", e)
	}
	// Forward the caller's DB-client environment (PGPASSWORD, PGSSLMODE,
	// MYSQL_PWD…) so credentials set on the host reach the tool; podman run
	// otherwise inherits none of the host env.
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if strings.HasPrefix(k, "PG") || strings.HasPrefix(k, "MYSQL") || strings.HasPrefix(k, "MARIADB") {
			runFlags = append(runFlags, "-e", e)
		}
	}
	runFlags = append(runFlags, image, "-c", shellCmd, "sh")
	runFlags = append(runFlags, args...)

	cmd := podman.Cmd(runFlags...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	return nil
}
