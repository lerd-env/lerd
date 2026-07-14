package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"gopkg.in/yaml.v3"
)

// The store-fetched Laravel definitions must keep declaring restart_command and
// tune_command on the queue worker: without them QueueRestartForSite no-ops and
// queue:start drops its tuned flags, since no Go merger backfills the fields.
func TestLaravelStoreQueueWorker_HasRestartAndTuneCommands(t *testing.T) {
	dir := filepath.Join("..", "..", "lerd-frameworks", "frameworks", "laravel")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("laravel store checkout not present: %v", err)
	}
	checked := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var fw config.Framework
		if err := yaml.Unmarshal(b, &fw); err != nil {
			t.Fatalf("unmarshal %s: %v", e.Name(), err)
		}
		q, ok := fw.Workers["queue"]
		if !ok {
			continue
		}
		checked++
		if q.RestartCommand == "" {
			t.Errorf("%s: queue worker missing restart_command", e.Name())
		}
		if q.TuneCommand == "" {
			t.Errorf("%s: queue worker missing tune_command", e.Name())
		}
	}
	if checked == 0 {
		t.Skip("no laravel version declares a queue worker")
	}
}

func TestRenderQueueCommand(t *testing.T) {
	// Laravel declares --queue=/--tries=/--timeout= flags.
	laravel := config.FrameworkWorker{
		Command:     "php artisan queue:work --queue=default --tries=3 --timeout=60",
		TuneCommand: "php artisan queue:work --queue={queue} --tries={tries} --timeout={timeout}",
	}
	if got := renderQueueCommand(laravel, "emails", 5, 120); got != "php artisan queue:work --queue=emails --tries=5 --timeout=120" {
		t.Errorf("laravel: got %q", got)
	}

	// CodeIgniter takes the queue positionally and has no per-job timeout flag.
	ci4 := config.FrameworkWorker{
		Command:     "php spark queue:work default",
		TuneCommand: "php spark queue:work {queue} -tries={tries}",
	}
	if got := renderQueueCommand(ci4, "emails", 5, 120); got != "php spark queue:work emails -tries=5" {
		t.Errorf("codeigniter: got %q", got)
	}

	// No template: fall back to the plain command verbatim.
	plain := config.FrameworkWorker{Command: "php spark queue:work default"}
	if got := renderQueueCommand(plain, "emails", 5, 120); got != "php spark queue:work default" {
		t.Errorf("fallback: got %q", got)
	}
}

func TestBuildHorizonUnit_AlwaysDependsOnRedis(t *testing.T) {
	unit := buildHorizonUnit("example-horizon", "/home/u/example-horizon", "lerd-php84-fpm")

	mustContain(t, unit, "Description=Lerd Horizon (example-horizon)")
	mustContain(t, unit, "After=network.target lerd-php84-fpm.service lerd-redis.service")
	mustContain(t, unit, "Wants=lerd-php84-fpm.service lerd-redis.service")
	mustContain(t, unit, "BindsTo=lerd-php84-fpm.service")
	mustContain(t, unit, "ExecStart="+podman.PodmanBin()+" exec -w '/home/u/example-horizon' lerd-php84-fpm php artisan horizon")
}

func mustContain(t *testing.T, body, needle string) {
	t.Helper()
	if !strings.Contains(body, needle) {
		t.Errorf("expected unit body to contain %q\n--- unit ---\n%s", needle, body)
	}
}
