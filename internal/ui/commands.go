package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/logcolor"
	"github.com/geodro/lerd/internal/plugins"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/sitetpl"
)

// runLocks holds a per-site mutex so two browser tabs (or the palette + the
// dropdown) can't fire the same destructive command concurrently. Keyed by
// site domain; the value is the locked flag plus a name for debugging.
var (
	runLocksMu sync.Mutex
	runLocks   = map[string]string{} // domain → currently-running command name
)

func tryAcquireRun(domain, name string) (release func(), busyWith string, ok bool) {
	runLocksMu.Lock()
	defer runLocksMu.Unlock()
	if cur, busy := runLocks[domain]; busy {
		return nil, cur, false
	}
	runLocks[domain] = name
	return func() {
		runLocksMu.Lock()
		delete(runLocks, domain)
		runLocksMu.Unlock()
	}, "", true
}

// siteRunLockKey derives the per-site run-lock key: prefer the name, fall back to
// the first domain, then the project path, so every site has a unique key shared
// by the command runner and the doctor-fix runner (they must exclude each other).
func siteRunLockKey(site *config.Site) string {
	if site.Name != "" {
		return site.Name
	}
	if len(site.Domains) > 0 {
		return site.Domains[0]
	}
	return site.Path
}

// commandRoute dispatches the commands subroutes:
//
//	GET  /api/sites/{domain}/commands              → list
//	POST /api/sites/{domain}/commands/{name}/run   → execute + stream
//	POST /api/sites/{domain}/commands/{name}/pin   → pin/unpin on the control row
//
// Returns true if the request was a commands subroute (handled here), false
// otherwise so the caller can fall through to the generic site action handler.
func commandRoute(w http.ResponseWriter, r *http.Request, domain string, rest []string) bool {
	if len(rest) == 0 || rest[0] != "commands" {
		return false
	}
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		writeJSON(w, map[string]any{"error": "site not found: " + domain})
		return true
	}
	switch {
	case len(rest) == 1 && r.Method == http.MethodGet:
		// Listing commands does not mutate the host.
		handleCommandsList(w, r, site)
	case len(rest) == 3 && rest[2] == "pin" && r.Method == http.MethodPost:
		// Pinning only rewrites the site registry, but it is still a host-config
		// mutation, so it takes the same authority the run route does.
		if !hasHostActionAuthority(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return true
		}
		handleCommandPin(w, r, site, rest[1])
	case len(rest) == 3 && rest[2] == "run" && r.Method == http.MethodPost:
		// Running a command requires dashboard-control authority because it
		// executes arbitrary shell code as the lerd-ui user.
		if !hasHostActionAuthority(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return true
		}
		handleCommandRun(w, r, site, rest[1])
	default:
		http.NotFound(w, r)
	}
	return true
}

func handleCommandsList(w http.ResponseWriter, r *http.Request, site *config.Site) {
	branch := r.URL.Query().Get("branch")
	cmds := config.ApplyCommandPins(resolveSiteCommands(site, branch), site.PinnedCommands)
	// A project-supplied command that hasn't been approved yet runs on the host,
	// so force the confirm modal (which shows the command) until the user approves
	// it once. ProjectOrigin itself is not serialized; the UI only sees Confirm.
	for i := range cmds {
		if cmds[i].ProjectOrigin && !site.CommandApproved(cmds[i].Command) {
			cmds[i].Confirm = true
		}
	}
	writeJSON(w, map[string]any{"commands": cmds})
}

// handleCommandPin toggles whether a command draws its own button on the site's
// control row. ?on=0 unpins; anything else pins. The cap is enforced here rather
// than only in the UI, so a second tab can't push the row past it.
func handleCommandPin(w http.ResponseWriter, r *http.Request, site *config.Site, name string) {
	on := r.URL.Query().Get("on") != "0"
	cmds := config.ApplyCommandPins(resolveSiteCommands(site, r.URL.Query().Get("branch")), site.PinnedCommands)
	var target *config.FrameworkCommand
	for i := range cmds {
		if cmds[i].Name == name {
			target = &cmds[i]
			break
		}
	}
	if target == nil {
		writeJSON(w, map[string]any{"error": "command not found: " + name})
		return
	}
	if on && !target.Pinned && config.CountPinned(cmds) >= config.MaxPinnedCommands {
		writeJSON(w, map[string]any{"error": fmt.Sprintf("at most %d pinned commands per site", config.MaxPinnedCommands)})
		return
	}
	if err := config.SetSiteCommandPinned(site.Name, name, on); err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// resolveSiteCommands merges the framework's command set with the project's
// .lerd.yaml entries. When `branch` is non-empty, resolves from the
// worktree's path so the worktree's .lerd.yaml overrides (or extras)
// take precedence over the main checkout's.
func resolveSiteCommands(site *config.Site, branch string) []config.FrameworkCommand {
	if site == nil {
		return nil
	}
	path := site.Path
	if branch != "" {
		if wt := resolveSitePath(site, branch); wt != "" {
			path = wt
		}
	}
	fw, _ := config.GetFrameworkForDir(site.Framework, path)
	proj, _ := config.LoadProjectConfig(path)
	cmds := sitetpl.ExpandCommands(config.ResolveCommands(fw, proj, path), sitetpl.ForPath(path))
	return append(cmds, pluginSiteCommands()...)
}

// pluginSiteCommands lists every installed plugin's site commands, named
// <plugin>:<command> so they cannot shadow a framework command.
func pluginSiteCommands() []config.FrameworkCommand {
	list, _ := plugins.Load()
	var out []config.FrameworkCommand
	for _, p := range list {
		for _, c := range p.SiteCommands {
			out = append(out, config.FrameworkCommand{
				Name:        p.Name + ":" + c.Name,
				Label:       c.Label,
				Description: c.Description,
				Confirm:     c.Confirm,
				Command:     filepath.Join(p.Dir, c.Run),
				Plugin:      p.Name,
			})
		}
	}
	return out
}

// runPluginCommand streams a plugin site command like any other, under the same
// per-site run lock. A plugin is installed by the user, so it needs no approval.
func runPluginCommand(w http.ResponseWriter, r *http.Request, site *config.Site, target *config.FrameworkCommand, branch string) {
	cwd := site.Path
	if branch != "" {
		if cwd = resolveSitePath(site, branch); cwd == "" {
			writeJSON(w, map[string]any{"error": "unknown worktree branch: " + branch})
			return
		}
	}
	release, busyWith, ok := tryAcquireRun(siteRunLockKey(site), target.Name)
	if !ok {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{"error": "another command is already running on this site: " + busyWith})
		return
	}
	defer release()
	cmd, err := pluginCommandProcess(target, site, branch, cwd)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), pluginSiteCommandLimit(target))
	defer cancel()
	code := streamRun(w, ctx, cmd, false)
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		plugins.Failed(plugins.Failure{Plugin: target.Plugin, Surface: plugins.SurfaceSiteCommand, What: target.Name, Error: fmt.Sprintf("timed out after %s", pluginSiteCommandLimit(target))})
	case code != 0:
		plugins.Failed(plugins.Failure{Plugin: target.Plugin, Surface: plugins.SurfaceSiteCommand, What: target.Name, Error: fmt.Sprintf("exited with code %d", code)})
	}
}

// pluginSiteCommandLimit is how long a plugin site command may run.
func pluginSiteCommandLimit(target *config.FrameworkCommand) time.Duration {
	if p, err := plugins.Find(target.Plugin); err == nil {
		for _, c := range p.SiteCommands {
			if target.Name == p.Name+":"+c.Name {
				return c.Limit()
			}
		}
	}
	return plugins.SiteCommandLimit
}

// pluginCommandProcess builds the process for a plugin site command, run in cwd
// with the site handed over the way every plugin surface does it.
func pluginCommandProcess(target *config.FrameworkCommand, site *config.Site, branch, cwd string) (*exec.Cmd, error) {
	p, err := plugins.Find(target.Plugin)
	if err != nil {
		return nil, err
	}
	run, err := filepath.Rel(p.Dir, target.Command)
	if err != nil {
		return nil, err
	}
	payload := plugins.SitePayload(site)
	payload["branch"] = branch
	cmd, err := p.Command(run, plugins.SurfaceSiteCommand, nil, plugins.Env(payload))
	if err != nil {
		return nil, err
	}
	cmd.Dir = cwd
	return cmd, nil
}

// urlRegex matches the first http/https URL in stdout, used when the command
// declares output: url (e.g. drush uli, wp user create-session-token).
var urlRegex = regexp.MustCompile(`https?://[^\s'"]+`)

// handleCommandRun executes the named command in the site's project directory
// and streams stdout+stderr to the client as Server-Sent Events. Frames:
//
//	event: stdout    data: <single line of output>
//	event: stderr    data: <single line of output>
//	event: done      data: {"exit": 0, "durationMs": 1234, "url": "..."}
//	event: error     data: <message>     (when setup fails before exec)
//
// stderr is interleaved into the same stream so the UI can render a unified
// terminal view, with a separate event type if we later want to colour it.
func handleCommandRun(w http.ResponseWriter, r *http.Request, site *config.Site, name string) {
	branch := r.URL.Query().Get("branch")
	cmds := resolveSiteCommands(site, branch)
	var target *config.FrameworkCommand
	for i := range cmds {
		if cmds[i].Name == name {
			target = &cmds[i]
			break
		}
	}
	if target == nil {
		writeJSON(w, map[string]any{"error": "command not found: " + name})
		return
	}
	if target.Command == "" {
		writeJSON(w, map[string]any{"error": "command has no shell invocation"})
		return
	}
	if target.Plugin != "" {
		runPluginCommand(w, r, site, target, branch)
		return
	}

	// A project-supplied command runs on the host. Require the user to have
	// approved this exact command (the confirm modal posts approve=1) before
	// running it; trusted framework commands are unaffected.
	if target.ProjectOrigin {
		allowed, disabled := config.HostCommandAllowed(site.Name, target.Command)
		switch {
		case disabled:
			writeJSON(w, map[string]any{"error": "project-supplied host commands are disabled (host_commands.disabled)"})
			return
		case !allowed && r.URL.Query().Get("approve") != "1":
			writeJSON(w, map[string]any{"needsConfirm": true, "command": target.Command})
			return
		case !allowed:
			_ = config.ApproveSiteCommand(site.Name, target.Command)
		}
	}

	// Per-site mutex: refuse if another command is already running on this
	// site. Prevents two tabs (or palette + dropdown) from concurrently
	// hammering migrate:fresh, etc.
	release, busyWith, ok := tryAcquireRun(siteRunLockKey(site), target.Name)
	if !ok {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{"error": "another command is already running on this site: " + busyWith})
		return
	}
	defer release()

	// Worktree branch must resolve; falling back to site.Path would point
	// destructive commands (migrate:fresh) at the main DB.
	basePath := site.Path
	if branch != "" {
		wt := resolveSitePath(site, branch)
		if wt == "" {
			writeJSON(w, map[string]any{"error": "unknown worktree branch: " + branch})
			return
		}
		basePath = wt
	}
	cwd := basePath
	if target.CWD != "" && target.CWD != "." {
		cwd = filepath.Join(basePath, target.CWD)
	}

	// Terminal mode: spawn the user's terminal emulator with the command
	// running inside, then return immediately. The UI handles this by
	// skipping the modal and showing a toast.
	if target.Output == config.CommandOutputTerminal {
		script := terminalCommandScript(cwd, target.Command)
		if err := openTerminalCommand(script); err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"terminal": true})
		return
	}

	streamShellRun(w, r.Context(), cwd, target.Command, target.Output == config.CommandOutputURL)
}

// streamShellRun execs shell in cwd and streams stdout/stderr to the client as
// SSE (event: stdout|stderr), closing with a done frame carrying the exit code,
// duration, and (when captureURL) a URL parsed from the output. Shared by the
// command runner and the doctor fix runner so both produce an identical stream.
func streamShellRun(w http.ResponseWriter, ctx context.Context, cwd, shell string, captureURL bool) {
	cmd := exec.Command("sh", "-c", shell)
	cmd.Dir = cwd
	// Force colour on: the command runs against a pipe, so composer, artisan
	// and friends would otherwise strip their ANSI escapes and the run modal
	// would show a wall of grey.
	cmd.Env = logcolor.Environ(append(os.Environ(), "PATH="+config.PathWithBinDir()))
	streamRun(w, ctx, cmd, captureURL)
}

// streamRun runs cmd and streams it as streamShellRun describes, killing it
// when the client goes away.
func streamRun(w http.ResponseWriter, ctx context.Context, cmd *exec.Cmd, captureURL bool) int {
	send, ok := sseSender(w)
	if !ok {
		return 0
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		send("error", "stdout pipe: "+err.Error())
		return 0
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		send("error", "stderr pipe: "+err.Error())
		return 0
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		send("error", "start: "+err.Error())
		return 0
	}
	defer context.AfterFunc(ctx, func() { _ = cmd.Process.Kill() })()

	// capturedMu guards the builder across the two pipe-reader goroutines;
	// strings.Builder is not safe for concurrent use. The frame writes are
	// serialized by the sender itself.
	var capturedMu sync.Mutex
	var captured strings.Builder
	streamPipe := func(pipe io.Reader, event string) {
		s := bufio.NewScanner(pipe)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			line := s.Text()
			capturedMu.Lock()
			captured.WriteString(line)
			captured.WriteByte('\n')
			capturedMu.Unlock()
			send(event, line)
		}
	}

	// defer-recover so a panic in streamPipe still releases the wait;
	// otherwise the handler would hang holding the per-site run lock.
	done := make(chan struct{}, 2)
	pump := func(pipe io.Reader, event string) {
		defer func() {
			_ = recover()
			done <- struct{}{}
		}()
		streamPipe(pipe, event)
	}
	go pump(stdout, "stdout")
	go pump(stderr, "stderr")
	<-done
	<-done

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			send("error", "wait: "+err.Error())
			return 0
		}
	}

	payload := map[string]any{
		"exit":       exitCode,
		"durationMs": time.Since(start).Milliseconds(),
	}
	if captureURL {
		if u := urlRegex.FindString(captured.String()); u != "" {
			payload["url"] = u
		}
	}
	body, _ := json.Marshal(payload)
	send("done", string(body))
	return exitCode
}

// sseSender opens the event stream a run is reported through and returns the
// frame writer for it. Writes are serialized because a run's stdout and stderr
// are pumped by separate goroutines, and interleaved bytes corrupt a frame.
func sseSender(w http.ResponseWriter) (func(event, data string), bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, map[string]any{"error": "streaming not supported"})
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx proxy buffering
	w.WriteHeader(http.StatusOK)

	var writeMu sync.Mutex
	return func(event, data string) {
		// Each non-empty data line must be prefixed; multi-line bodies use
		// multiple `data:` lines per the SSE spec.
		var b strings.Builder
		b.WriteString("event: ")
		b.WriteString(event)
		b.WriteByte('\n')
		for _, line := range strings.Split(data, "\n") {
			b.WriteString("data: ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
		writeMu.Lock()
		defer writeMu.Unlock()
		_, _ = io.WriteString(w, b.String())
		flusher.Flush()
	}, true
}

// streamHostAction reports a fix lerd carries out on the host, rather than in
// the site's container, through the same stream a command run speaks, so the
// run modal shows its outcome like any other.
func streamHostAction(w http.ResponseWriter, line string, actionErr error) {
	send, ok := sseSender(w)
	if !ok {
		return
	}
	exit := 0
	if actionErr != nil {
		send("stderr", actionErr.Error())
		exit = 1
	} else {
		send("stdout", line)
	}
	body, _ := json.Marshal(map[string]any{"exit": exit, "durationMs": 0})
	send("done", string(body))
}

// terminalCommandScript is what the spawned terminal runs: the command in the
// project directory, then a pause so the window holds its output.
//
// The trap is what keeps ctrl+c usable. A non-interactive shell dies on SIGINT
// with the command it is waiting on, so interrupting a long-running one left
// the terminal reporting a crashed shell instead of the command's own goodbye
// and the pause below. It is a handler rather than an ignore on purpose: an
// ignored SIGINT is inherited by every child, and the command would stop
// answering ctrl+c at all.
func terminalCommandScript(cwd, command string) string {
	return "trap 'true' INT\n" +
		"cd " + podman.ShellQuote(cwd) + " && " + command +
		"\nprintf '\\n[press any key to close]'\nread -n 1 -s -r 2>/dev/null || read"
}
