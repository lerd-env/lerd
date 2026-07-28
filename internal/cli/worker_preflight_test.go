package cli

import (
	"os"
	"path/filepath"
	"strings"
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

// A host node worker with Node unmanaged and nothing resolvable (no system
// node/npm, no bun) is held back with an actionable message instead of being
// started into a crash loop — issue #1143.
func TestWorkerStartPreflight_noUsableNodeIsActionable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("PATH", filepath.Join(tmp, "empty"))
	// The developer's own shell exports NVM_DIR, and this test is describing a
	// host with no Node at all.
	t.Setenv("NVM_DIR", filepath.Join(tmp, "home", ".nvm"))

	sitePath := t.TempDir()
	os.WriteFile(filepath.Join(sitePath, "package.json"), []byte("{}"), 0644)

	w := config.FrameworkWorker{Command: "npm run dev", Host: true}
	err := workerStartPreflight(sitePath, "vite", w)
	if err == nil {
		t.Fatal("expected a hold-back error when no Node is resolvable")
	}
	for _, want := range []string{"vite worker not started", "not managing Node", "lerd install"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %s", want, err)
		}
	}

	// A non-Node command in the same repo is unaffected.
	direct := config.FrameworkWorker{Command: "./server --port 8000", Host: true}
	if err := workerStartPreflight(sitePath, "app", direct); err != nil {
		t.Errorf("non-Node command should pass: %v", err)
	}
}
