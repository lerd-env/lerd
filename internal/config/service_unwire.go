package config

import (
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/envfile"
)

// UnwireProjectService points a project's env file away from a service. The
// keys the framework wires for it go back to the example file's values, and
// any key still naming the lerd-<service> container is cleared when the example
// has nothing for it. Keys the file lacks are never added, and driver keys such
// as CACHE_STORE=redis are left alone: lerd cannot know what to switch them to.
func UnwireProjectService(dir, service string) error {
	file, format := EnvFileFor(dir)
	path := filepath.Join(dir, file)
	current := envfile.Values(path, format)

	fw := frameworkFor(dir)
	mapped := map[string]bool{}
	var example map[string]string
	if fw != nil {
		for _, kv := range fw.Env.Services[service].Vars {
			k, _, _ := strings.Cut(kv, "=")
			mapped[k] = true
		}
		if ex := fw.Env.ExampleFile; ex != "" && ex != file {
			example = envfile.Values(filepath.Join(dir, ex), format)
		}
	}

	updates := map[string]string{}
	for k, v := range current {
		pointsAtService := envfile.ReferencesContainer(v, service)
		if !mapped[k] && !pointsAtService {
			continue
		}
		exv, inExample := example[k]
		if inExample && !envfile.ReferencesContainer(exv, service) {
			if exv != v {
				updates[k] = exv
			}
			continue
		}
		if pointsAtService {
			updates[k] = ""
		}
	}
	if len(updates) == 0 {
		return nil
	}
	return envfile.ApplyUpdatesIn(path, format, updates)
}
