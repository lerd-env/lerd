package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"gopkg.in/yaml.v3"
)

// The store-fetched Laravel definitions must keep declaring restart_command,
// tune_command and the Redis requirement on the queue worker: without them
// QueueRestartForSite no-ops, queue:start loses its flags, and a redis queue
// with lerd-redis down goes back to failing on a DNS error, since no Go merger
// backfills any of the three.
func TestLaravelStoreQueueWorker_HasRestartTuneAndService(t *testing.T) {
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
		if q.RequiresService == nil || q.RequiresService.Name != "redis" ||
			q.RequiresService.WhenEnv != "QUEUE_CONNECTION=redis" {
			t.Errorf("%s: queue worker missing requires_service redis on QUEUE_CONNECTION=redis", e.Name())
		}
	}
	if checked == 0 {
		t.Skip("no laravel version declares a queue worker")
	}
}

var laravelQueue = config.FrameworkWorker{
	Command:     "php artisan queue:work --queue=default --tries=3 --timeout=60",
	TuneCommand: "php artisan queue:work --queue={queue} --tries={tries} --timeout={timeout}",
}

// CodeIgniter takes the queue positionally and never spells a tries default, so
// its template can only be matched up to the first placeholder.
var codeigniterQueue = config.FrameworkWorker{
	Command:     "php spark queue:work default",
	TuneCommand: "php spark queue:work {queue} -tries={tries}",
}

func TestWorkerTuneFlags(t *testing.T) {
	t.Run("defaults come back from the plain command", func(t *testing.T) {
		want := []workerTuneFlag{
			{Name: "queue", Default: "default"},
			{Name: "tries", Default: "3"},
			{Name: "timeout", Default: "60"},
		}
		got := workerTuneFlags(laravelQueue)
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("flag %d: got %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("a placeholder the command does not spell has no default", func(t *testing.T) {
		got := workerTuneFlags(codeigniterQueue)
		if len(got) != 2 {
			t.Fatalf("got %v, want two flags", got)
		}
		if got[0].Name != "queue" || got[0].Default != "default" {
			t.Errorf("queue flag: got %v", got[0])
		}
		if got[1].Name != "tries" || got[1].Default != "" {
			t.Errorf("tries flag: got %v, want no default", got[1])
		}
	})

	t.Run("a worker without a tune command declares no flags", func(t *testing.T) {
		if got := workerTuneFlags(config.FrameworkWorker{Command: "php artisan schedule:work"}); got != nil {
			t.Errorf("got %v, want none", got)
		}
	})
}

func TestRenderTuneCommand(t *testing.T) {
	t.Run("nothing overridden runs the declared command", func(t *testing.T) {
		got, err := renderTuneCommand(laravelQueue, nil)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if got != laravelQueue.Command {
			t.Errorf("got %q, want %q", got, laravelQueue.Command)
		}
	})

	t.Run("an override keeps the other defaults", func(t *testing.T) {
		got, err := renderTuneCommand(laravelQueue, map[string]string{"queue": "emails"})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		want := "php artisan queue:work --queue=emails --tries=3 --timeout=60"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("a framework with its own syntax renders that syntax", func(t *testing.T) {
		got, err := renderTuneCommand(codeigniterQueue, map[string]string{"queue": "emails", "tries": "5"})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		want := "php spark queue:work emails -tries=5"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("a placeholder with no default and no value is refused", func(t *testing.T) {
		_, err := renderTuneCommand(codeigniterQueue, map[string]string{"queue": "emails"})
		if err == nil || !strings.Contains(err.Error(), "--tries") {
			t.Fatalf("got %v, want an error naming --tries", err)
		}
	})

	t.Run("whitespace in a value is refused", func(t *testing.T) {
		_, err := renderTuneCommand(laravelQueue, map[string]string{"queue": "a b"})
		if err == nil || !strings.Contains(err.Error(), "whitespace") {
			t.Fatalf("got %v, want a whitespace error", err)
		}
	})

	t.Run("a worker without a tune command is never rewritten", func(t *testing.T) {
		plain := config.FrameworkWorker{Command: "php spark queue:work default"}
		got, err := renderTuneCommand(plain, map[string]string{"queue": "emails"})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if got != plain.Command {
			t.Errorf("got %q, want %q", got, plain.Command)
		}
	})
}
