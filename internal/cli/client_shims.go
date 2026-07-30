package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/feedback"
	phpDet "github.com/geodro/lerd/internal/php"
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
		// The glued short form is "-hHOST" with no whitespace; "-h note" (a space)
		// is a -c/-e body that happens to start with -h, not a host flag.
		gluedHost := strings.HasPrefix(a, "-h") && len(a) > 2 && !strings.ContainsAny(a, " \t")
		if a == "-h" || a == "--host" || strings.HasPrefix(a, "--host=") || gluedHost {
			return true
		}
		// A connection URI (scheme://…) or a libpq conninfo string carries its own
		// host. Conninfo detection requires an actual "host" key, so a host= inside a
		// SQL body (via -c/-e) or a "--where=host=x" filter is never mistaken for one.
		if isConnURI(a) || looksLikeConninfo(a) {
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

// looksLikeConninfo reports whether a is a libpq conninfo string that names a
// host: every whitespace-separated token is a key=value pair AND one token's key
// is exactly "host". A SQL query fails the first test on its first bare word
// (SELECT, WHERE…); a spaceless "--where=host=x" filter passes that but fails the
// host-key test, since its only key is "--where". So neither is read as a host.
func looksLikeConninfo(a string) bool {
	fields := strings.Fields(a)
	if len(fields) == 0 {
		return false
	}
	hasHost := false
	for _, f := range fields {
		i := strings.IndexByte(f, '=')
		if i <= 0 {
			return false
		}
		if f[:i] == "host" {
			hasHost = true
		}
	}
	return hasHost
}

// loopbackHosts are the spellings of "this machine" a database client accepts
// as -h/--host. An explicit host naming one of these is not automatically an
// external database: it may be how the caller reaches the very port a lerd
// service published on the host, which resolveLoopbackTarget below detects.
var loopbackHosts = map[string]bool{
	"127.0.0.1": true,
	"::1":       true,
	"localhost": true,
}

// hostFlagSpan returns the value of an explicit -h/--host flag in args and
// the [start, start+n) index span it occupies (n=2 for a separate-value
// form, 1 for a glued/= form), so a matched loopback host can be stripped
// back out. Detection mirrors argsSpecifyHost's flag-form cases; conninfo
// strings and connection URIs are intentionally not parsed here; they carry
// host and port together and are left to pass through untouched.
func hostFlagSpan(args []string) (value string, start, n int, ok bool) {
	for i, a := range args {
		switch {
		case a == "-h" || a == "--host":
			if i+1 < len(args) {
				return args[i+1], i, 2, true
			}
		case strings.HasPrefix(a, "--host="):
			return strings.TrimPrefix(a, "--host="), i, 1, true
		case strings.HasPrefix(a, "-h") && len(a) > 2 && !strings.ContainsAny(a, " \t"):
			return a[2:], i, 1, true
		}
	}
	return "", 0, 0, false
}

// portFlagSpan is hostFlagSpan for the tool family's explicit port flag.
// Postgres tools use -p/--port; mysql tools use -P/--port, since mysql's
// lowercase -p is --password, not --port.
func portFlagSpan(tool string, args []string) (value string, start, n int, ok bool) {
	short := "-p"
	if !isPostgresTool(tool) {
		short = "-P"
	}
	for i, a := range args {
		switch {
		case a == short || a == "--port":
			if i+1 < len(args) {
				return args[i+1], i, 2, true
			}
		case strings.HasPrefix(a, "--port="):
			return strings.TrimPrefix(a, "--port="), i, 1, true
		case strings.HasPrefix(a, short) && len(a) > len(short):
			return a[len(short):], i, 1, true
		}
	}
	return "", 0, 0, false
}

// defaultToolPort is the family's standard port, used to find a loopback
// match when the caller names a host but no explicit port (the client's own
// default would apply, e.g. psql defaulting to 5432).
func defaultToolPort(tool string) string {
	if isPostgresTool(tool) {
		return "5432"
	}
	return "3306"
}

// loopbackServiceOwningPort returns the installed service exposing tool whose
// published host port matches hostPort, disambiguating among several
// same-family instances (e.g. postgres-18 on 5432 vs. postgres-timescaledb on
// 5433) the way ResolveTarget's single "owner" can't.
func loopbackServiceOwningPort(tool, hostPort string) (string, bool) {
	port, err := strconv.Atoi(hostPort)
	if err != nil {
		return "", false
	}
	for _, name := range shims.ToolCandidates(tool) {
		if slices.Contains(config.HostPortsFor(name), port) {
			return name, true
		}
	}
	return "", false
}

// loopbackServiceOwningPortFn is the seam resolveLoopbackTarget calls through,
// so a test can drive its arg-rewriting logic with a fake match instead of a
// real installed-service/config fixture.
var loopbackServiceOwningPortFn = loopbackServiceOwningPort

// resolveLoopbackTarget rewrites args when they name an explicit loopback
// host (127.0.0.1/localhost/::1) whose port matches one lerd's own installed
// services actually publishes. That combination isn't an external database:
// it's the port `lerd service start` printed, reached the normal way — but
// runClientExec always execs the client tool inside a throwaway container on
// its own network, where "127.0.0.1" is the container's own loopback, not
// the host's. Passing such args straight through (the general "external host"
// behaviour) therefore always fails with connection refused. Recognise the
// case and route it exactly like a hostless call instead: strip the loopback
// -h/-p pair and let the caller add "-h lerd-<service>" at the container's
// internal (family-default) port. Returns the original args unchanged, with
// hostGiven=true, when the host isn't a loopback spelling or matches no
// installed service (a genuinely external host, left untouched as before).
func resolveLoopbackTarget(tool string, args []string) (out []string, hostGiven bool, prefer string) {
	host, hStart, hN, ok := hostFlagSpan(args)
	if !ok {
		return args, false, ""
	}
	if !loopbackHosts[host] {
		return args, true, ""
	}
	port, pStart, pN, hasPort := portFlagSpan(tool, args)
	if !hasPort {
		port = defaultToolPort(tool)
	}
	svc, matched := loopbackServiceOwningPortFn(tool, port)
	if !matched {
		return args, true, ""
	}
	drop := make([]bool, len(args))
	for i := hStart; i < hStart+hN; i++ {
		drop[i] = true
	}
	if hasPort {
		for i := pStart; i < pStart+pN; i++ {
			drop[i] = true
		}
	}
	rewritten := make([]string, 0, len(args))
	for i, a := range args {
		if !drop[i] {
			rewritten = append(rewritten, a)
		}
	}
	return rewritten, false, svc
}

// hostEnvSet reports whether the caller pointed the tool at a host via the
// family's environment variable, which also counts as an explicit target.
func hostEnvSet(tool string) bool {
	if isPostgresTool(tool) {
		return os.Getenv("PGHOST") != ""
	}
	return os.Getenv("MYSQL_HOST") != ""
}

// effectiveHostGiven folds the family's environment-variable host (PGHOST /
// MYSQL_HOST) into resolveLoopbackTarget's args-based decision, the way a
// real client would: an explicit -h flag always governs, so the env var is
// only consulted when args named no host at all. Without this guard, an
// unrelated PGHOST left set in the shell would silently re-flag a call
// resolveLoopbackTarget already matched and rewrote to a local lerd service
// (prefer != "") as external again, discarding that rewrite's effect even
// though the -h flag that triggered it should win.
func effectiveHostGiven(tool string, argHostGiven bool, prefer string) bool {
	if prefer != "" || argHostGiven {
		return argHostGiven
	}
	return hostEnvSet(tool)
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

// isVersionProbe reports whether the arguments are a bare version or help query,
// which is how an IDE validates a client executable before offering it. The
// postgres tools answer one only as their very first argument.
func isVersionProbe(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "--version", "-V", "--help", "-?":
		return true
	}
	return false
}

// wantsLocalDefault reports whether the shim should aim a hostless invocation at
// the backing lerd service. A version probe is excluded: prepending -h in front
// of it leaves pg_dump exiting non-zero with no version, and the IDE that asked
// then rejects the shim path.
func wantsLocalDefault(tool string, args []string, hostGiven bool) bool {
	return !hostGiven && isSQLTool(tool) && !isVersionProbe(args)
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
	host := envfile.ReadKey(filepath.Join(dbEnvRootFor(cwd), ".env"), "DB_HOST")
	svc, ok := strings.CutPrefix(host, "lerd-")
	if !ok || svc == "" {
		return ""
	}
	return svc
}

// dbEnvRootFor returns the directory whose .env describes the database a command
// run from cwd talks to. A worktree nested inside its parent site matches that
// site by path, so reading the site root would route a branch at the parent's DB.
func dbEnvRootFor(cwd string) string {
	if wt, _, ok := phpDet.WorktreeRootFor(cwd); ok {
		if _, err := os.Stat(filepath.Join(wt, ".env")); err == nil {
			return wt
		}
	}
	return phpDet.SiteRootFor(cwd)
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
	// back to the global owner. An explicit host is normally external and passes
	// straight through — except a loopback host matching one of lerd's own
	// published ports, which resolveLoopbackTarget rewrites to route (and
	// strips) like a hostless call; see its doc comment for why the pass-through
	// path can never reach that case on its own.
	rewritten, argHostGiven, prefer := resolveLoopbackTarget(tool, args)
	args = rewritten
	hostGiven := effectiveHostGiven(tool, argHostGiven, prefer)
	if prefer == "" && !hostGiven {
		prefer = siteServiceForTool(cwd, tool)
	}
	target, ok := shims.ResolveTarget(tool, prefer)
	if !ok {
		return fmt.Errorf("no installed service exposes the %q client tool", tool)
	}

	// Autostart the backing service so its image is present and, for a dump
	// against a local lerd database (-h lerd-<service>), the server is up. A
	// version probe needs neither, so it skips the start once the image is there.
	image := podman.InstalledImage("lerd-" + target.Service)
	if image == "" || !isVersionProbe(args) {
		if err := ensureServiceRunning(target.Service); err != nil {
			return fmt.Errorf("could not start %s: %w", target.Service, err)
		}
		image = podman.InstalledImage("lerd-" + target.Service)
	}
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
	if wantsLocalDefault(tool, args, hostGiven) {
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
