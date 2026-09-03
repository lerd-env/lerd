package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/composer"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

var (
	ensurePathMounted = podman.EnsurePathMounted
	pathAutoMountable = podman.PathAutoMountable
	pathVisible       = podman.PathVisible
)

// NewNewCmd returns the new command — scaffold a new PHP project.
func NewNewCmd() *cobra.Command {
	var frameworkName string
	var frameworkVersion string

	cmd := &cobra.Command{
		Use:   "new [name-or-path]",
		Short: "Scaffold a new PHP project and take it through link and setup",
		Long: `Create a new PHP project using the framework's scaffold command.

On a terminal this asks which framework to use, offering what the store
publishes and which major to scaffold, then carries the project through link
and setup so it ends up served. Name a framework and that question is skipped;
run without a terminal and the questions are too, so scripts keep working.

  lerd new                                # ask for the name, framework and version
  lerd new myapp                          # ask which framework to use
  lerd new myapp --framework=symfony      # scaffold Symfony, no questions
  lerd new myapp --framework=laravel --framework-version=11   # scaffold an older major
  lerd new /path/to/myapp                 # create at an absolute path
  lerd new myapp -- --no-interaction      # pass extra args to the scaffold command

Flags anywhere on the line belong to lerd; everything after '--' is handed to
the scaffold command untouched.

Every framework's scaffold command comes from its YAML definition:
  create: composer create-project myvendor/myframework`,
		Args:                  cobra.ArbitraryArgs,
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, extraArgs := newArgs(args, cmd.ArgsLenAtDash())
			return runNew(target, frameworkName, frameworkVersion, extraArgs)
		},
	}

	cmd.Flags().StringVar(&frameworkName, "framework", "",
		"Framework to scaffold; asked on a terminal when omitted, "+defaultScaffoldFramework+" otherwise")
	cmd.Flags().StringVar(&frameworkVersion, "framework-version", "",
		"Major to scaffold; requires --framework, defaults to the latest the store publishes")

	return cmd
}

// newVersionNeedsFramework reports a version with no framework to apply it to.
// The wizard would ask which framework and the typed version would be dropped
// against whatever came back, so the flag pair is refused instead.
func newVersionNeedsFramework(frameworkName, frameworkVersion string) bool {
	return frameworkVersion != "" && frameworkName == ""
}

// newArgs splits the command line into the target and the arguments to hand the
// scaffold command. dash is cobra's ArgsLenAtDash: the count of arguments before
// a literal `--`, or -1 when there was none. It is what tells `lerd new -- --x`,
// which names no project, from `lerd new myapp -- --x`, which names one, since
// both arrive as a plain list.
func newArgs(args []string, dash int) (string, []string) {
	if dash == 0 {
		return "", args
	}
	if len(args) == 0 {
		return "", nil
	}
	return args[0], args[1:]
}

// newNextStep builds the post-scaffold hint, preserving the path the user
// typed (filepath.Base would drop the parent dirs of a nested target). A run
// that already carried the project through link and setup has nothing left to
// suggest but the one thing the command cannot do, which is move the user's own
// shell into the new directory.
func newNextStep(typedTarget string, chained bool) string {
	if chained {
		return "cd " + typedTarget
	}
	return "cd " + typedTarget + " && lerd link && lerd setup"
}

// prepareScaffoldParent creates the target's parent directory and makes it
// visible inside the PHP container. The scaffold shells out to composer, which
// is a container shim, so an unmounted parent leaves crun with nothing to chdir
// into and it exits 127 before composer ever runs.
func prepareScaffoldParent(target string) error {
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("cannot create %s: %w", parent, err)
	}
	cfg, _ := config.LoadGlobal()
	version := cfg.PHP.DefaultVersion
	if !pathVisible(parent, version) && !pathAutoMountable(parent) {
		return fmt.Errorf("cannot scaffold into %s: lerd does not mount temporary system directories (/tmp, /var/tmp, /run) into containers, so composer would have no such directory to run in. Pick a path under your home directory, or park the parent first with 'lerd park %s'", parent, parent)
	}
	ensurePathMounted(parent, version)
	return nil
}

// scaffold is how a framework's create command will be run: either through the
// composer lerd ships (inside the project's PHP container) or, for a create
// command composer cannot serve, a plain host binary.
type scaffold struct {
	inContainer bool
	args        []string
}

// scaffoldPlan turns a framework's create command into the argument list that
// runs it. Every definition in the store starts with composer, which lerd
// bundles as a phar rather than expecting on the host, so that prefix is
// swapped for the bundled one and run with the container's PHP. Anything else
// is left to the host binary it names.
func scaffoldPlan(create, target string, extraArgs []string) scaffold {
	parts := strings.Fields(create)
	if len(parts) == 0 {
		return scaffold{}
	}
	tail := append(append([]string{}, parts[1:]...), target)
	tail = append(tail, extraArgs...)

	if parts[0] != "composer" {
		return scaffold{args: append([]string{parts[0]}, tail...)}
	}
	return scaffold{
		inContainer: true,
		args:        append([]string{composer.PharPath()}, tail...),
	}
}

// scaffoldPHPVersion returns the PHP version the scaffold should run under: the
// machine default clamped into the framework's declared range, so composer
// resolves against a PHP the framework supports rather than whatever the parent
// directory (empty, so the default) would pick. Returns "" when the framework
// declares no range, leaving the caller on the default as before.
func scaffoldPHPVersion(fw *config.Framework) string {
	if fw.PHP.Min == "" && fw.PHP.Max == "" {
		return ""
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return ""
	}
	return phpDet.ClampToRange(cfg.PHP.DefaultVersion, fw.PHP.Min, fw.PHP.Max)
}

// runScaffold executes a scaffold plan from the target's parent directory. When
// version is set, the containerized create command runs under that PHP rather
// than the one the parent directory resolves to.
func runScaffold(plan scaffold, workDir, version string) error {
	if plan.inContainer {
		var (
			code int
			err  error
		)
		if version != "" {
			code, err = RunPHPVersionCaptureEnv(workDir, version, plan.args, []string{composer.ProcessTimeoutEnv()})
		} else {
			code, err = RunPHPCaptureEnv(workDir, plan.args, []string{composer.ProcessTimeoutEnv()})
		}
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("exit status %d", code)
		}
		return nil
	}

	cmd := exec.Command(plan.args[0], plan.args[1:]...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runNew(target, frameworkName, frameworkVersion string, extraArgs []string) error {
	interactive := isInteractive()

	if newVersionNeedsFramework(frameworkName, frameworkVersion) {
		return fmt.Errorf("--framework-version needs a --framework to apply it to")
	}

	// Ask for what the command was not told. A terminal gets the catalogue and
	// the majors published for whatever it picks; anything else keeps the
	// long-standing default so a script never starts blocking on a prompt.
	if target == "" {
		if !interactive {
			return fmt.Errorf("give the project a name: lerd new <name-or-path>")
		}
		answer, err := askProjectName()
		if err != nil {
			return err
		}
		target = answer
	}
	if newShouldAskFramework(interactive, frameworkName != "") {
		name, version, err := askScaffoldFramework(scaffoldCatalogue())
		if err != nil {
			return err
		}
		frameworkName, frameworkVersion = name, version
	}
	if frameworkName == "" {
		frameworkName = defaultScaffoldFramework
	}

	// Preserve the path as typed for the "Next" hint before resolving it.
	typedTarget := target

	// Resolve target to an absolute path
	if !filepath.IsAbs(target) {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		target = filepath.Join(cwd, target)
	}

	// Look up the framework in the store, so a new project starts from the
	// published definition rather than whichever snapshot of it this binary was
	// built with. Falls back to the local definition when the store is unreachable.
	fw, ok := config.GetFrameworkForScaffold(frameworkName, frameworkVersion)
	if !ok {
		return fmt.Errorf("unknown framework %q — run 'lerd framework list' to see available frameworks", frameworkName)
	}
	if fw.Create == "" {
		return fmt.Errorf("framework %q has no create command — add a 'create' field to its YAML definition", frameworkName)
	}

	if err := prepareScaffoldParent(target); err != nil {
		return err
	}

	plan := scaffoldPlan(fw.Create, target, extraArgs)
	if len(plan.args) == 0 {
		return fmt.Errorf("framework %q has an empty create command", frameworkName)
	}

	scaffoldVersion := scaffoldPHPVersion(fw)

	start := time.Now()
	feedback.Begin()
	line := "scaffolding " + feedback.Val(fw.Label) + " · " + strings.Join(strings.Fields(fw.Create), " ") + " " + target
	if plan.inContainer && scaffoldVersion != "" {
		line += " · php " + feedback.Val(scaffoldVersion)
	}
	feedback.Line(line)
	fmt.Println()

	if err := runScaffold(plan, filepath.Dir(target), scaffoldVersion); err != nil {
		return fmt.Errorf("scaffold command failed: %w", err)
	}

	feedback.Success("created "+filepath.Base(target), time.Since(start))

	// Take the project the rest of the way. The link routes a project with no
	// .lerd.yaml through the init wizard (PHP version, HTTPS, services) and
	// offers setup at the end, so scaffolding lands on a served site rather than
	// three commands the user has to know about.
	chained := false
	if interactive {
		chained = true
		if err := inDir(target, func() error { return runLinkOrInit(nil) }); err != nil {
			feedback.Warn("link: %v", err)
		}
	}

	feedback.NewSummary().
		Row("Path", target).
		Row("Next", newNextStep(typedTarget, chained)).
		Print()
	return nil
}

// inDir runs fn with the process working directory moved to dir, restoring it
// afterwards. The chained steps each resolve the project from os.Getwd, and this
// is the one caller that has to point them somewhere other than where the user
// ran the command, so it moves once here rather than threading a directory
// through link, init and setup.
func inDir(dir string, fn func() error) error {
	prev, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(prev) }()
	return fn()
}
