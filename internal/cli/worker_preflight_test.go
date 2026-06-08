package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestWorkerStartPreflight_RejectsInjectionCommand(t *testing.T) {
	// A worker command from .lerd.yaml custom_workers reaches the unit's
	// ExecStart line; a newline would inject an extra systemd directive.
	bad := config.FrameworkWorker{Command: "php artisan queue:work\nExecStartPost=/bin/sh -c pwned"}
	if err := workerStartPreflight("/srv/x", "queue", bad); err == nil {
		t.Fatal("expected rejection of a newline-bearing worker command")
	}
	badReload := config.FrameworkWorker{Command: "ok", ReloadCommand: "reload\nExecStart=evil"}
	if err := workerStartPreflight("/srv/x", "queue", badReload); err == nil {
		t.Fatal("expected rejection of a newline-bearing reload command")
	}
	// A clean command passes the injection check (no Check rules => nil).
	if err := workerStartPreflight("/srv/x", "queue", config.FrameworkWorker{Command: "php artisan queue:work"}); err != nil {
		t.Fatalf("clean command should pass: %v", err)
	}
}
