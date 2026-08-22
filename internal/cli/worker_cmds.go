package cli

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// The named worker commands — queue:start, horizon:reload, reverb:stop and
// whatever a framework declares next — are generated here from the workers the
// current project's framework defines, over the same implementation `lerd worker
// start` uses. A worker added to the store therefore arrives with its own
// commands, and no framework name is spelled out in Go.

// NewFrameworkWorkerCmds returns the commands for every worker the current
// directory's framework declares: a `<name>` parent with start/stop (plus
// reload where the definition has a reload variant) and the `<name>:start`
// spellings alongside it. Empty outside a linked site, which is also the only
// place the commands could do anything.
func NewFrameworkWorkerCmds() []*cobra.Command {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	return workerCmdsFor(cwd, frameworkWorkersForCwd())
}

// workerCmdsFor builds the commands for a set of workers. A worker whose check
// rule fails here keeps its commands but stays out of the help listing: the
// command is what explains which package is missing, so removing it would leave
// `lerd reverb:start` answering "unknown command" instead.
func workerCmdsFor(cwd string, workers map[string]config.FrameworkWorker) []*cobra.Command {
	names := make([]string, 0, len(workers))
	for name := range workers {
		names = append(names, name)
	}
	sort.Strings(names)

	var cmds []*cobra.Command
	for _, name := range names {
		w := workers[name]
		hidden := cwd != "" && w.Check != nil && !config.MatchesRule(cwd, *w.Check)
		parent := &cobra.Command{
			Use:    name,
			Short:  "Manage " + workerDisplayName(name, w) + " for the current site",
			Hidden: hidden,
		}
		parent.AddCommand(newGeneratedWorkerStartCmd("start", name, w))
		parent.AddCommand(newGeneratedWorkerStopCmd("stop", name, w))
		group := []*cobra.Command{parent,
			newGeneratedWorkerStartCmd(name+":start", name, w),
			newGeneratedWorkerStopCmd(name+":stop", name, w)}
		if w.ReloadCommand != "" {
			parent.AddCommand(newGeneratedWorkerReloadCmd("reload", name, w))
			group = append(group, newGeneratedWorkerReloadCmd(name+":reload", name, w))
		}
		for _, c := range group {
			c.Hidden = hidden
			cmds = append(cmds, c)
		}
	}
	return cmds
}

// WantsFrameworkWorkerCmds reports whether this invocation needs the generated
// commands built. Resolving them reads the framework definition and can refresh
// it from the store over the network, so an invocation that names a statically
// registered command skips the work entirely; what is left is help and
// completion, which list everything, and a first argument no static command
// answers to, which is the only thing a worker command can look like.
func WantsFrameworkWorkerCmds(root *cobra.Command, args []string) bool {
	first := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			first = a
			break
		}
	}
	if first == "" {
		return slices.Contains(args, "-h") || slices.Contains(args, "--help")
	}
	switch first {
	case "help", "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return true
	}
	for _, c := range root.Commands() {
		if c.Name() == first || slices.Contains(c.Aliases, first) {
			return false
		}
	}
	return true
}

// frameworkWorkersForCwd resolves the workers declared for the current
// directory without touching anything: an unlinked directory is left unlinked
// rather than registered on the way to building a command tree.
func frameworkWorkersForCwd() map[string]config.FrameworkWorker {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	site, err := config.FindSiteByPath(cwd)
	if err != nil {
		if parent, ok := config.ParentSiteForWorktreeDir(cwd); ok {
			site = parent
		} else {
			// A freshly cloned project is not registered yet, but its .lerd.yaml
			// names the framework, so its worker commands exist before `lerd link`
			// and answer with that command's own error rather than "unknown
			// command".
			return workersForFramework(projectFrameworkName(cwd), cwd)
		}
	}
	if site.IsCustomContainer() && site.Framework == "" {
		proj, _ := config.LoadProjectConfig(cwd)
		if proj == nil {
			return nil
		}
		return proj.CustomWorkers
	}
	return workersForFramework(site.Framework, cwd)
}

func projectFrameworkName(cwd string) string {
	proj, err := config.LoadProjectConfig(cwd)
	if err != nil || proj == nil {
		return ""
	}
	return proj.Framework
}

func workersForFramework(name, cwd string) map[string]config.FrameworkWorker {
	fw, ok := config.GetFrameworkForDir(name, cwd)
	if !ok {
		return nil
	}
	return fw.Workers
}

// workerDisplayName is the worker's label, falling back to its name.
func workerDisplayName(name string, w config.FrameworkWorker) string {
	if w.Label != "" {
		return w.Label
	}
	return name
}

func newGeneratedWorkerStartCmd(use, name string, w config.FrameworkWorker) *cobra.Command {
	flags := workerTuneFlags(w)
	values := make(map[string]*string, len(flags))

	cmd := &cobra.Command{
		Use:   use,
		Short: "Start " + workerDisplayName(name, w) + " for the current site as a systemd service",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			// Only flags the user actually passed count as tuning: with none of
			// them touched the worker runs the command its definition declares,
			// exactly as `lerd worker start` would.
			tuned := map[string]string{}
			for _, f := range flags {
				if c.Flags().Changed(f.Name) {
					tuned[f.Name] = *values[f.Name]
				}
			}
			return runWorkerStart(name, tuned)
		},
	}
	for _, f := range flags {
		v := new(string)
		cmd.Flags().StringVar(v, f.Name, f.Default, "Value substituted for {"+f.Name+"} in the worker command")
		values[f.Name] = v
	}
	return cmd
}

func newGeneratedWorkerStopCmd(use, name string, w config.FrameworkWorker) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: "Stop " + workerDisplayName(name, w) + " for the current site",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return runWorkerStop(name) },
	}
}

// newGeneratedWorkerReloadCmd toggles auto-reload mode on or off for a worker
// whose definition declares a reload variant. With no argument it prints the
// current state. When toggled it persists the per-project preference and, if the
// worker is running, restarts it so the new command takes effect immediately.
func newGeneratedWorkerReloadCmd(use, name string, w config.FrameworkWorker) *cobra.Command {
	return &cobra.Command{
		Use:   use + " [on|off]",
		Short: "Toggle auto-reload on file changes (" + w.ReloadCommand + ") for the current site",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			site, fw, phpVersion, err := resolveSiteAndFramework(cwd)
			if err != nil {
				return err
			}
			def, err := frameworkWorkerFrom(fw, name)
			if err != nil {
				return err
			}
			if def.ReloadCommand == "" {
				return fmt.Errorf("worker %q has no reload variant in framework %q", name, fw.Label)
			}

			if len(args) == 0 {
				state := "off"
				if config.ProjectReloadsWorker(cwd, name) {
					state = "on"
				}
				fmt.Printf("%s auto-reload (%s): %s\n", workerDisplayName(name, def), def.ReloadCommand, state)
				return nil
			}

			enable, err := parseOnOff(args[0])
			if err != nil {
				return err
			}
			if err := ApplyWorkerReload(site.Name, cwd, phpVersion, name, enable); err != nil {
				return err
			}

			feedback.Begin()
			if enable {
				feedback.Done(workerDisplayName(name, def) + " auto-reload enabled")
				feedback.Note("the worker restarts on file changes")
			} else {
				feedback.Done(workerDisplayName(name, def) + " auto-reload disabled")
			}
			return nil
		},
	}
}

func parseOnOff(arg string) (bool, error) {
	switch strings.ToLower(arg) {
	case "on", "true", "1", "enable", "enabled":
		return true, nil
	case "off", "false", "0", "disable", "disabled":
		return false, nil
	}
	return false, fmt.Errorf("expected 'on' or 'off', got %q", arg)
}

// runWorkerStart is the one start path behind `lerd worker start <name>` and
// every generated `<name>:start`. tuned carries the placeholder values the user
// overrode, empty when none were.
func runWorkerStart(name string, tuned map[string]string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	site, fw, phpVersion, err := resolveSiteAndFramework(cwd)
	if err != nil {
		return err
	}
	w, err := frameworkWorkerFrom(fw, name)
	if err != nil {
		return err
	}
	command, err := renderTuneCommand(w, tuned)
	if err != nil {
		return err
	}
	w.Command = command

	if err := WorkerStartForSite(site.Name, cwd, phpVersion, name, w, true); err != nil {
		return err
	}
	if !site.Paused {
		_ = config.SetProjectWorkers(site.Path, CollectRunningWorkerNames(site))
	}
	return nil
}

// runWorkerStop is the one stop path behind `lerd worker stop <name>` and every
// generated `<name>:stop`. A worker missing from the definition can still be
// stopped when its unit is running, so a store change never strands a process.
func runWorkerStop(name string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	site, fw, _, err := resolveSiteAndFramework(cwd)
	if err != nil {
		return err
	}
	if _, ok := fw.Workers[name]; !ok {
		if !isServiceActiveOrRestarting(WorkerUnitName(site.Name, cwd, name)) {
			return errUnknownWorker(fw, name)
		}
	}
	if err := WorkerStopForSite(site.Name, cwd, name); err != nil {
		return err
	}
	if !site.Paused {
		_ = config.SetProjectWorkers(site.Path, CollectRunningWorkerNames(site))
	}
	return nil
}

func errUnknownWorker(fw *config.Framework, name string) error {
	return fmt.Errorf("framework %q has no worker named %q\nRun 'lerd worker list' to see available workers", fw.Label, name)
}

func frameworkWorkerFrom(fw *config.Framework, name string) (config.FrameworkWorker, error) {
	w, ok := fw.Workers[name]
	if !ok {
		return config.FrameworkWorker{}, errUnknownWorker(fw, name)
	}
	return w, nil
}

// StartFrameworkWorker starts the named worker for a site from its framework
// definition. The entry point for callers outside the CLI (the web UI, the
// install restore), so none of them has to know what any particular worker is.
func StartFrameworkWorker(siteName, sitePath, phpVersion, workerName string) error {
	return StartFrameworkWorkerTuned(siteName, sitePath, phpVersion, workerName, nil)
}

// StartFrameworkWorkerTuned starts the worker with values substituted into its
// tune_command placeholders. An empty map runs the plain command.
func StartFrameworkWorkerTuned(siteName, sitePath, phpVersion, workerName string, values map[string]string) error {
	fw, ok := config.GetFrameworkForDir(siteFrameworkName(siteName), sitePath)
	if !ok {
		return fmt.Errorf("no framework found for site %q", siteName)
	}
	w, err := frameworkWorkerFrom(fw, workerName)
	if err != nil {
		return err
	}
	command, err := renderTuneCommand(w, values)
	if err != nil {
		return err
	}
	w.Command = command
	return WorkerStartForSite(siteName, sitePath, phpVersion, workerName, w, true)
}

// StopFrameworkWorker stops the named worker for a site.
func StopFrameworkWorker(siteName, workerName string) error {
	return WorkerStopForSite(siteName, "", workerName)
}

// ApplyWorkerReload persists the per-project auto-reload preference for a worker
// and, when that worker is currently running for the site, restarts it so the
// new command takes effect immediately. The restart goes back through
// StartFrameworkWorker, which resolves the command (standard or reload variant)
// from the freshly persisted preference.
func ApplyWorkerReload(siteName, sitePath, phpVersion, workerName string, enabled bool) error {
	// Refuse to enable when the watcher prerequisite is missing rather than
	// persisting a preference the worker can't honour. Without this the toggle
	// would read "on" while resolveWorkerCommand quietly ran the standard
	// command, so the displayed state would diverge from reality.
	if enabled && !projectHasChokidar(sitePath) {
		return fmt.Errorf("%s auto-reload needs the chokidar npm package, which is not installed in this project\nInstall it with: npm install -D chokidar\n(Vite 8 no longer ships it, so a plain npm install is not enough)", workerName)
	}
	if err := config.SetProjectWorkerReload(sitePath, workerName, enabled); err != nil {
		return err
	}
	if !workerRunningForSite(siteName, workerName) {
		return nil
	}
	if err := StopFrameworkWorker(siteName, workerName); err != nil {
		return fmt.Errorf("stop %s: %w", workerName, err)
	}
	if err := StartFrameworkWorker(siteName, sitePath, phpVersion, workerName); err != nil {
		return fmt.Errorf("restart %s: %w", workerName, err)
	}
	return nil
}

// workerRunningForSite reports whether the named site currently runs the worker.
func workerRunningForSite(siteName, workerName string) bool {
	site, err := config.FindSite(siteName)
	if err != nil || site == nil {
		return false
	}
	return slices.Contains(CollectRunningWorkerNames(site), workerName)
}

// workerTuneFlag is one flag a worker declares through its tune_command.
type workerTuneFlag struct {
	Name    string
	Default string
}

var tunePlaceholderRe = regexp.MustCompile(`\{([a-zA-Z][a-zA-Z0-9_-]*)\}`)

// workerTuneFlags reads the flags a worker declares through its tune_command
// placeholders, in the order they appear. Each default is recovered by matching
// the template against the plain command, so --queue offers whatever the
// definition already runs without the value being repeated anywhere. A
// placeholder the plain command has no counterpart for gets no default and has
// to be passed.
func workerTuneFlags(w config.FrameworkWorker) []workerTuneFlag {
	if w.TuneCommand == "" {
		return nil
	}
	var names, segments []string
	rest := w.TuneCommand
	for {
		loc := tunePlaceholderRe.FindStringSubmatchIndex(rest)
		if loc == nil {
			segments = append(segments, rest)
			break
		}
		segments = append(segments, rest[:loc[0]])
		names = append(names, rest[loc[2]:loc[3]])
		rest = rest[loc[1]:]
	}

	flags := make([]workerTuneFlag, 0, len(names))
	pos := 0
	for i, name := range names {
		var def string
		def, pos = tuneDefault(w.Command, segments[i], segments[i+1], pos)
		flags = append(flags, workerTuneFlag{Name: name, Default: def})
	}
	return flags
}

// tuneDefault recovers one placeholder's value from the plain command by
// locating the literal text the template puts around it. Returns "" and -1 when
// that text isn't there, which ends the scan: without a fixed point the later
// placeholders can't be located either.
func tuneDefault(command, before, after string, from int) (string, int) {
	if from < 0 || from > len(command) {
		return "", -1
	}
	idx := strings.Index(command[from:], before)
	if idx < 0 {
		return "", -1
	}
	start := from + idx + len(before)
	if after == "" {
		return command[start:], len(command)
	}
	end := strings.Index(command[start:], after)
	if end < 0 {
		// The template says more than the plain command does (CodeIgniter takes
		// the queue positionally and never spells its -tries= default), so the
		// rest of the command is this placeholder's value and the ones after it
		// have no default.
		return command[start:], len(command)
	}
	return command[start : start+end], start + end
}

// renderTuneCommand substitutes values into the worker's tune_command. With
// nothing overridden the plain command is returned verbatim, so a start with no
// flags runs exactly what the definition declares.
func renderTuneCommand(w config.FrameworkWorker, values map[string]string) (string, error) {
	if w.TuneCommand == "" || len(values) == 0 {
		return w.Command, nil
	}
	command := w.TuneCommand
	for _, f := range workerTuneFlags(w) {
		value := values[f.Name]
		if value == "" {
			value = f.Default
		}
		if value == "" {
			return "", fmt.Errorf("--%s must be given: the framework definition declares no default for {%s}", f.Name, f.Name)
		}
		// The value is interpolated into the command the worker's unit runs, so
		// whitespace or a newline could add an argument or a systemd directive.
		if strings.ContainsAny(value, " \t\r\n") {
			return "", fmt.Errorf("invalid --%s value: must not contain whitespace", f.Name)
		}
		command = strings.ReplaceAll(command, "{"+f.Name+"}", value)
	}
	return command, nil
}
