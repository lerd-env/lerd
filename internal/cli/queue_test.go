package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

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
	mustContain(t, unit, "ExecStart="+podman.PodmanBin()+" exec -w /home/u/example-horizon lerd-php84-fpm php artisan horizon")
}

func mustContain(t *testing.T, body, needle string) {
	t.Helper()
	if !strings.Contains(body, needle) {
		t.Errorf("expected unit body to contain %q\n--- unit ---\n%s", needle, body)
	}
}
