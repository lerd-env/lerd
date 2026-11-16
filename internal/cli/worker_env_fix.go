package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/sitedoctor"
)

// startGatedWorker starts a worker the worker_env fix just ungated, against the
// site or, for a worktree, the worktree itself. A var so tests skip systemd.
var startGatedWorker = func(site config.Site, path, php, name string, w config.FrameworkWorker) error {
	if path == site.Path {
		return StartFrameworkWorker(site.Name, site.Path, php, name)
	}
	return WorkerStartForSite(site.Name, path, php, name, w, false)
}

// ApplyWorkerEnv writes the .env value each fixable gated worker at path needs
// and starts those that run there, per_worktree ones only in a worktree. It
// returns what it did, empty when there was nothing to fix.
func ApplyWorkerEnv(site config.Site, path string) (string, error) {
	fw, _ := config.GetFrameworkForDir(site.Framework, path)
	updates := map[string]string{}
	var workers []string
	for _, g := range sitedoctor.WorkerEnvGaps(path, fw) {
		if g.Service == "" {
			continue
		}
		updates[g.Key] = g.Want
		workers = append(workers, g.Worker)
	}
	if len(updates) == 0 {
		return "", nil
	}
	// .env, not the framework's env file: it is the file when_env is read from.
	if err := envfile.ApplyUpdates(filepath.Join(path, ".env"), updates); err != nil {
		return "", fmt.Errorf("writing .env: %w", err)
	}
	php := site.PHPVersion
	if path != site.Path {
		php = config.WorktreePHPVersion(path, site.PHPVersion)
	}
	var started []string
	for _, name := range workers {
		w := fw.Workers[name]
		if path != site.Path && !w.IsPerWorktree() {
			continue
		}
		if err := startGatedWorker(site, path, php, name, w); err != nil {
			return "set the .env value", fmt.Errorf("starting %s: %w", name, err)
		}
		started = append(started, name)
	}
	if len(started) == 0 {
		return "set the .env value", nil
	}
	return "set the .env value and started " + strings.Join(started, ", "), nil
}
