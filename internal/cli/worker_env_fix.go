package cli

import (
	"fmt"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/sitedoctor"
)

// ApplyWorkerEnv resolves the worker_env finding for the checkout at path: it
// writes the .env value each fixable gated worker needs, then starts those
// workers. A worktree only gets the .env write, its workers start with it.
func ApplyWorkerEnv(site config.Site, path string) ([]string, error) {
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
		return nil, nil
	}
	// .env, not the framework's env file: it is the file when_env is read from.
	if err := envfile.ApplyUpdates(filepath.Join(path, ".env"), updates); err != nil {
		return nil, fmt.Errorf("writing .env: %w", err)
	}
	if path != site.Path {
		return workers, nil
	}
	for _, w := range workers {
		if err := StartFrameworkWorker(site.Name, site.Path, site.PHPVersion, w); err != nil {
			return workers, fmt.Errorf("starting %s: %w", w, err)
		}
	}
	return workers, nil
}
