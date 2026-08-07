package sitedoctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// drupalish declares a primary env file lerd writes and a fallback published so
// an already-installed project can be read, which is Drupal's shape.
func drupalish() *config.Framework {
	return &config.Framework{
		Name:  "drupalish",
		Label: "Drupalish",
		Env: config.FrameworkEnvConf{
			File:           ".env",
			Format:         "dotenv",
			FallbackFile:   "web/sites/default/settings.php",
			FallbackFormat: "php-const",
			Services: map[string]config.FrameworkServiceDef{
				"mysql": {Vars: []string{"DB_HOST=lerd-mysql", "DB_NAME={{site}}"}},
			},
		},
	}
}

// A project read through its fallback still has no env file of its own, and
// reporting the fallback as present hides the one lerd and the framework's own
// install command both need.
func TestCheckEnvPresent_reportsTheMissingPrimaryBehindAReadableFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web", "sites", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "web/sites/default/settings.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp := Run(context.Background(), dir, drupalish())

	for _, c := range resp.Checks {
		if c.Name != "env_present" {
			continue
		}
		if c.Status != StatusFail || !strings.Contains(c.Detail, ".env") {
			t.Errorf("env_present = %+v, want a failure naming .env", c)
		}
		return
	}
	t.Fatal("no env_present check in the report")
}

func TestCheckEnvPresent_passesOnceThePrimaryExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST=lerd-mysql\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp := Run(context.Background(), dir, drupalish())

	for _, c := range resp.Checks {
		if c.Name == "env_present" && c.Status != StatusOK {
			t.Errorf("env_present = %+v, want ok", c)
		}
	}
}
