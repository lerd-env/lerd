package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestRunNotifyTarget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := runNotifyTarget("bogus"); err == nil {
		t.Fatal("expected an error for an invalid target")
	}

	if err := runNotifyTarget(config.NotifyTargetNative); err != nil {
		t.Fatalf("setting native: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NotificationTarget() != config.NotifyTargetNative {
		t.Fatalf("target=%q, want native", cfg.NotificationTarget())
	}

	if err := runNotifyTarget(config.NotifyTargetBrowser); err != nil {
		t.Fatalf("setting browser: %v", err)
	}
	cfg, _ = config.LoadGlobal()
	if cfg.NotificationTarget() != config.NotifyTargetBrowser {
		t.Fatalf("target=%q, want browser", cfg.NotificationTarget())
	}
}
