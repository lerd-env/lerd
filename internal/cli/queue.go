package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// NewQueueCmd returns the queue parent command with start/stop subcommands.
func NewQueueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Manage queue workers for the current site",
	}
	cmd.AddCommand(newQueueStartCmd("start"))
	cmd.AddCommand(newQueueStopCmd("stop"))
	return cmd
}

// NewQueueStartCmd returns the standalone queue:start command.
func NewQueueStartCmd() *cobra.Command { return newQueueStartCmd("queue:start") }

// NewQueueStopCmd returns the standalone queue:stop command.
func NewQueueStopCmd() *cobra.Command { return newQueueStopCmd("queue:stop") }

func newQueueStartCmd(use string) *cobra.Command {
	var queue string
	var tries int
	var timeout int

	cmd := &cobra.Command{
		Use:   use,
		Short: "Start a queue worker for the current site as a systemd service",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runQueueStart(queue, tries, timeout)
		},
	}
	cmd.Flags().StringVar(&queue, "queue", "default", "Queue name to process")
	cmd.Flags().IntVar(&tries, "tries", 3, "Number of times to attempt a job before logging it as failed")
	cmd.Flags().IntVar(&timeout, "timeout", 60, "Seconds a job may run before timing out")
	return cmd
}

func newQueueStopCmd(use string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: "Stop the queue worker for the current site",
		RunE:  func(_ *cobra.Command, _ []string) error { return runQueueStop() },
	}
}

func queueSiteName(cwd string) (string, error) {
	reg, err := config.LoadSites()
	if err != nil {
		return "", err
	}
	for _, s := range reg.Sites {
		if s.Path == cwd {
			return s.Name, nil
		}
	}
	// Fall back to directory name.
	name, _ := siteops.SiteNameAndDomain(filepath.Base(cwd), "test")
	return name, nil
}

func runQueueStart(queue string, tries, timeout int) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if err := requireFrameworkWorker(cwd, "queue"); err != nil {
		return err
	}

	siteName, err := queueSiteName(cwd)
	if err != nil {
		return err
	}

	phpVersion, err := phpDet.DetectVersion(cwd)
	if err != nil {
		cfg, _ := config.LoadGlobal()
		phpVersion = cfg.PHP.DefaultVersion
	}

	if err := queueStartTuned(siteName, cwd, phpVersion, queue, tries, timeout); err != nil {
		return err
	}
	if site, err := config.FindSite(siteName); err == nil && !site.Paused {
		_ = config.SetProjectWorkers(site.Path, CollectRunningWorkerNames(site))
	}
	return nil
}

func runQueueStop() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if err := requireFrameworkWorker(cwd, "queue"); err != nil {
		return err
	}

	siteName, err := queueSiteName(cwd)
	if err != nil {
		return err
	}

	if err := QueueStopForSite(siteName); err != nil {
		return err
	}
	if site, err := config.FindSite(siteName); err == nil && !site.Paused {
		_ = config.SetProjectWorkers(site.Path, CollectRunningWorkerNames(site))
	}
	return nil
}

// renderQueueCommand substitutes {queue}/{tries}/{timeout} into the worker's
// TuneCommand template (each framework declares its own flag syntax), falling
// back to the plain Command when no template is defined.
func renderQueueCommand(w config.FrameworkWorker, queue string, tries, timeout int) string {
	if w.TuneCommand == "" {
		return w.Command
	}
	return strings.NewReplacer(
		"{queue}", queue,
		"{tries}", strconv.Itoa(tries),
		"{timeout}", strconv.Itoa(timeout),
	).Replace(w.TuneCommand)
}

// queueStartTuned starts the queue worker with a specific queue/tries/timeout by
// rendering the framework's TuneCommand. Used by `lerd queue:start` and, via the
// mcp.QueueStartFn hook, by the MCP queue_start tool.
func queueStartTuned(siteName, sitePath, phpVersion, queue string, tries, timeout int) error {
	// The queue name is interpolated into the worker command; whitespace or a
	// newline could inject extra arguments or a systemd directive.
	if strings.ContainsAny(queue, " \t\r\n") {
		return fmt.Errorf("invalid queue name: must not contain whitespace")
	}
	// Pre-flight: if the site uses Redis as its queue connection, make sure
	// lerd-redis is actually running. Without it the queue worker fails immediately
	// with a cryptic PHP "getaddrinfo for lerd-redis failed" DNS error.
	envPath := filepath.Join(sitePath, ".env")
	if envfile.ReadKey(envPath, "QUEUE_CONNECTION") == "redis" {
		if running, _ := podman.ContainerRunning("lerd-redis"); !running {
			return fmt.Errorf("queue worker requires Redis (QUEUE_CONNECTION=redis in .env) but lerd-redis is not running\nStart it first: lerd services start redis")
		}
	}

	fw, ok := config.GetFrameworkForDir(siteFrameworkName(siteName), sitePath)
	if !ok {
		return fmt.Errorf("no framework found for site %q", siteName)
	}
	worker, ok := fw.Workers["queue"]
	if !ok {
		return fmt.Errorf("framework %q has no worker named \"queue\"", fw.Label)
	}

	// Derive the command from the framework definition instead of hardcoding
	// Laravel's artisan syntax, so non-Laravel frameworks (e.g. CodeIgniter's
	// `php spark queue:work`) run their own worker command.
	workerCopy := worker
	workerCopy.Command = renderQueueCommand(worker, queue, tries, timeout)

	return WorkerStartForSite(siteName, sitePath, phpVersion, "queue", workerCopy, true)
}

// QueueStartForSite starts a queue worker for the given site using the command
// from the framework definition.
func QueueStartForSite(siteName, sitePath, phpVersion string) error {
	fw, ok := config.GetFrameworkForDir(siteFrameworkName(siteName), sitePath)
	if !ok {
		return fmt.Errorf("no framework found for site %q", siteName)
	}
	worker, ok := fw.Workers["queue"]
	if !ok {
		return fmt.Errorf("framework %q has no worker named \"queue\"", fw.Label)
	}
	return WorkerStartForSite(siteName, sitePath, phpVersion, "queue", worker, true)
}

// QueueRestartForSite gracefully restarts the queue worker by running the
// framework's RestartCommand in the FPM container. No-op when the site has no
// queue unit or the framework declares no restart command (e.g. CodeIgniter).
func QueueRestartForSite(siteName, sitePath, phpVersion string) error {
	unitFile := filepath.Join(config.SystemdUserDir(), "lerd-queue-"+siteName+".service")
	if _, err := os.Stat(unitFile); os.IsNotExist(err) {
		return nil
	}
	fw, ok := config.GetFrameworkForDir(siteFrameworkName(siteName), sitePath)
	if !ok {
		return nil
	}
	worker, ok := fw.Workers["queue"]
	if !ok || worker.RestartCommand == "" {
		return nil
	}
	// Heal legacy units: a graceful restart exits cleanly (code 0), which
	// Restart=on-failure would not respawn. Upgrade them to Restart=always.
	if data, err := os.ReadFile(unitFile); err == nil {
		if healed := strings.ReplaceAll(string(data), "Restart=on-failure", "Restart=always"); healed != string(data) {
			if err := os.WriteFile(unitFile, []byte(healed), 0644); err == nil {
				_ = podman.DaemonReloadFn()
			}
		}
	}
	if phpVersion == "" {
		cfg, _ := config.LoadGlobal()
		phpVersion = cfg.PHP.DefaultVersion
	}
	container := resolveWorkerFPMUnit(siteName, phpVersion)
	if container == "" {
		container = "lerd-php" + strings.ReplaceAll(phpVersion, ".", "") + "-fpm"
	}
	if running, _ := podman.ContainerRunning(container); !running {
		return nil
	}
	args := append([]string{"exec", "-w", sitePath, container}, strings.Fields(worker.RestartCommand)...)
	if _, err := podman.Run(args...); err != nil {
		return fmt.Errorf("queue restart for %s: %w", siteName, err)
	}
	fmt.Printf("Queue worker signaled to restart for %s\n", siteName)
	return nil
}

// QueueStopForSite stops and removes the queue worker for the named site.
func QueueStopForSite(siteName string) error {
	return WorkerStopForSite(siteName, "", "queue")
}
