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

	cmd := &cobra.Command{
		Use:   "new <name-or-path>",
		Short: "Scaffold a new PHP project",
		Long: `Create a new PHP project using the framework's scaffold command.

  lerd new myapp                          # create ./myapp using Laravel (default)
  lerd new myapp --framework=symfony      # create ./myapp using Symfony
  lerd new /path/to/myapp                 # create at an absolute path
  lerd new myapp -- --no-interaction      # pass extra args to the scaffold command

Flags anywhere on the line belong to lerd; everything after '--' is handed to
the scaffold command untouched.

For Laravel this runs:
  composer create-project --no-install --no-plugins --no-scripts laravel/laravel <target> [extra args]

Other frameworks must define a 'create' field in their YAML definition:
  create: composer create-project myvendor/myframework

After creation, register the site with:
  cd <target>
  lerd link
  lerd setup`,
		Args:                  cobra.MinimumNArgs(1),
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			extraArgs := args[1:]
			return runNew(target, frameworkName, extraArgs)
		},
	}

	cmd.Flags().StringVar(&frameworkName, "framework", "laravel", "Framework to use")

	return cmd
}

// newNextStep builds the post-scaffold hint, preserving the path the user
// typed (filepath.Base would drop the parent dirs of a nested target).
func newNextStep(typedTarget string) string {
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
		args:        append([]string{filepath.Join(config.BinDir(), "composer.phar")}, tail...),
	}
}

// runScaffold executes a scaffold plan from the target's parent directory.
func runScaffold(plan scaffold, workDir string) error {
	if plan.inContainer {
		code, err := RunPHPCaptureEnv(workDir, plan.args, []string{composer.ProcessTimeoutEnv()})
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

func runNew(target, frameworkName string, extraArgs []string) error {
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

	// Look up the framework
	fw, ok := config.GetFramework(frameworkName)
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

	start := time.Now()
	feedback.Begin()
	feedback.Line("scaffolding " + feedback.Val(fw.Label) + " · " + strings.Join(strings.Fields(fw.Create), " ") + " " + target)
	fmt.Println()

	if err := runScaffold(plan, filepath.Dir(target)); err != nil {
		return fmt.Errorf("scaffold command failed: %w", err)
	}

	feedback.Success("created "+filepath.Base(target), time.Since(start))
	feedback.NewSummary().
		Row("Path", target).
		Row("Next", newNextStep(typedTarget)).
		Print()
	return nil
}
