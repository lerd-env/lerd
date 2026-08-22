package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/spf13/cobra"
)

func cmdNames(cmds []*cobra.Command) []string {
	names := make([]string, 0, len(cmds))
	for _, c := range cmds {
		names = append(names, c.Name())
	}
	return names
}

func TestWorkerCmdsFor(t *testing.T) {
	workers := map[string]config.FrameworkWorker{
		"queue":    laravelQueue,
		"horizon":  {Command: "php artisan horizon", ReloadCommand: "php artisan horizon:listen"},
		"schedule": {Command: "php artisan schedule:work"},
	}
	cmds := workerCmdsFor("", workers)
	names := cmdNames(cmds)

	for _, want := range []string{
		"queue", "queue:start", "queue:stop",
		"horizon", "horizon:start", "horizon:stop", "horizon:reload",
		"schedule", "schedule:start", "schedule:stop",
	} {
		if !slices.Contains(names, want) {
			t.Errorf("missing generated command %q (got %v)", want, names)
		}
	}

	// Only a worker with a reload variant gets the reload spelling.
	if slices.Contains(names, "schedule:reload") {
		t.Errorf("schedule declares no reload_command but got a reload command: %v", names)
	}

	// The parent carries the same verbs as subcommands.
	for _, c := range cmds {
		if c.Name() != "horizon" {
			continue
		}
		if got := cmdNames(c.Commands()); !slices.Contains(got, "start") ||
			!slices.Contains(got, "stop") || !slices.Contains(got, "reload") {
			t.Errorf("horizon parent subcommands: got %v", got)
		}
	}
}

func TestWorkerCmdsFor_FlagsComeFromTheTuneCommand(t *testing.T) {
	cmds := workerCmdsFor("", map[string]config.FrameworkWorker{
		"queue":    codeigniterQueue,
		"schedule": {Command: "php artisan schedule:work"},
	})
	for _, c := range cmds {
		switch c.Name() {
		case "queue:start":
			if c.Flags().Lookup("queue") == nil || c.Flags().Lookup("tries") == nil {
				t.Error("queue:start should carry the --queue and --tries flags its template declares")
			}
			if c.Flags().Lookup("timeout") != nil {
				t.Error("queue:start should not carry --timeout: this framework's template has no such placeholder")
			}
			if got := c.Flags().Lookup("queue").DefValue; got != "default" {
				t.Errorf("--queue default: got %q, want %q", got, "default")
			}
		case "schedule:start":
			if c.Flags().HasFlags() {
				t.Error("a worker without a tune_command should have no tuning flags")
			}
		}
	}
}

func TestWantsFrameworkWorkerCmds(t *testing.T) {
	root := &cobra.Command{Use: "lerd"}
	root.AddCommand(&cobra.Command{Use: "status"})
	root.AddCommand(&cobra.Command{Use: "worker"})

	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"status"}, false},
		{[]string{"worker", "start", "queue"}, false},
		{[]string{"--verbose", "status"}, false},
		{[]string{"queue:start"}, true},
		{[]string{"horizon", "reload", "on"}, true},
		{[]string{"help"}, true},
		{[]string{"--help"}, true},
		{[]string{}, false},
	}
	for _, c := range cases {
		if got := WantsFrameworkWorkerCmds(root, c.args); got != c.want {
			t.Errorf("args %v: got %v, want %v", c.args, got, c.want)
		}
	}
}

func TestRequiredServiceFor(t *testing.T) {
	worker := config.FrameworkWorker{
		Command:         "php artisan queue:work",
		RequiresService: &config.WorkerService{Name: "redis", WhenEnv: "QUEUE_CONNECTION=redis"},
	}

	site := t.TempDir()
	if got := requiredServiceFor(site, worker); got != "" {
		t.Errorf("no .env: got %q, want none", got)
	}

	writeSiteEnv(t, site, "QUEUE_CONNECTION=database\n")
	if got := requiredServiceFor(site, worker); got != "" {
		t.Errorf("database queue: got %q, want none", got)
	}

	writeSiteEnv(t, site, "QUEUE_CONNECTION=redis\n")
	if got := requiredServiceFor(site, worker); got != "redis" {
		t.Errorf("redis queue: got %q, want %q", got, "redis")
	}

	unconditional := config.FrameworkWorker{RequiresService: &config.WorkerService{Name: "redis"}}
	if got := requiredServiceFor(t.TempDir(), unconditional); got != "redis" {
		t.Errorf("unconditional requirement: got %q, want %q", got, "redis")
	}

	if got := requiredServiceFor(site, config.FrameworkWorker{}); got != "" {
		t.Errorf("worker without a requirement: got %q, want none", got)
	}
}

func writeSiteEnv(t *testing.T, sitePath, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(sitePath, ".env"), []byte(body), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}
