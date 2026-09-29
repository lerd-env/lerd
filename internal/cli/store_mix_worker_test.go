package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"gopkg.in/yaml.v3"
)

// Laravel Mix outlived the move to vite: a project scaffolded on Mix keeps it
// across every later upgrade, so each Laravel major declares the watcher. It
// rebuilds into public/ where nginx already serves, so no dev server is proxied.
func TestStoreLaravel_DeclaresMixWorker(t *testing.T) {
	root := filepath.Join("..", "..", "lerd-frameworks", "frameworks", "laravel")
	files, err := filepath.Glob(filepath.Join(root, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Skipf("frameworks store checkout not present: %v", err)
	}
	for _, path := range files {
		name := filepath.Base(path)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var fw config.Framework
		if err := yaml.Unmarshal(b, &fw); err != nil {
			t.Fatalf("unmarshal %s: %v", name, err)
		}
		m, ok := fw.Workers["mix"]
		if !ok {
			t.Errorf("%s: no mix worker declared", name)
			continue
		}
		if !m.Host {
			t.Errorf("%s: mix worker must run on the host", name)
		}
		if m.PerWorktree == nil || !*m.PerWorktree {
			t.Errorf("%s: mix worker must be per_worktree", name)
		}
		if !m.ReplacesBuild {
			t.Errorf("%s: mix worker must set replaces_build", name)
		}
		if m.Command != "npm run watch" {
			t.Errorf("%s: mix worker command = %q, want npm run watch", name, m.Command)
		}
		if m.Check == nil || m.Check.File != "node_modules/laravel-mix" {
			t.Errorf("%s: mix worker must check for node_modules/laravel-mix", name)
		}
		if m.Health != nil && m.Health.URLFile != "" {
			t.Errorf("%s: mix worker watches files and serves nothing, so it has no url_file", name)
		}
	}
}
