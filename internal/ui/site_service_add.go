package ui

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteinfo"
)

// addSiteService takes up a service the dashboard suggested for a linked site:
// it lands in .lerd.yaml, then link installs and starts it and env wires it,
// the same two steps the setup wizard runs after its questions. preset and
// version answer it with another preset filling the role, or another version.
func addSiteService(site *config.Site, name, preset, version string) error {
	e := siteinfo.Enrich(*site, siteinfo.EnrichFramework|siteinfo.EnrichServices)
	if !slices.ContainsFunc(e.SuggestedServices, func(sg config.ServiceSuggestion) bool { return sg.Name == name }) {
		return fmt.Errorf("%q is not a service suggested for %s", name, site.Name)
	}
	svc, err := suggestedVariant(name, preset, version)
	if err != nil {
		return err
	}
	return wireSiteService(site, svc)
}

// wireSiteService puts svc in the site's .lerd.yaml and has link and env act on it.
func wireSiteService(site *config.Site, svc config.ProjectService) error {
	name := svc.Name
	if err := config.AddProjectServices(site.Path, []config.ProjectService{svc}); err != nil {
		return err
	}
	// Adding it is asking for it: link skips a service marked removed, which
	// would wire .env to a container that never comes up.
	if err := config.SetServiceRemoved(name, false); err != nil {
		return err
	}
	if err := config.SetSiteServiceDeclined(site.Name, name, false); err != nil {
		return err
	}
	for _, args := range [][]string{{"link", "--yes"}, {"env"}} {
		if out, err := runLerdFn(site.Path, args...); err != nil {
			return fmt.Errorf("lerd %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(lastLines(string(out), 5)))
		}
	}
	return nil
}

// suggestedVariant is the .lerd.yaml entry for answering the suggestion role
// with preset at version, both optional. A versioned pick is named the way the
// preset names that version's service, so link installs exactly that one.
func suggestedVariant(role, preset, version string) (config.ProjectService, error) {
	if preset == "" {
		preset = role
	}
	if preset != role || version != "" {
		// Ensure, not Load: the picker lists store presets this machine has not cached.
		p, err := config.EnsurePreset(preset)
		if err != nil {
			return config.ProjectService{}, err
		}
		if !p.FillsRole(role) {
			return config.ProjectService{}, fmt.Errorf("%q cannot stand in for %s", preset, role)
		}
		if version != "" {
			resolved, err := p.Resolve(version)
			if err != nil {
				return config.ProjectService{}, err
			}
			// The canonical version is the bare service every older entry names.
			if resolved.Name != preset {
				return config.ProjectService{Name: resolved.Name, Preset: preset, PresetVersion: version}, nil
			}
		}
	}
	if config.IsDefaultPreset(preset) {
		return config.ProjectService{Name: preset}, nil
	}
	return config.ProjectService{Name: preset, Preset: preset}, nil
}

// runLerdFn runs this lerd binary in dir; a seam so tests need no binary.
var runLerdFn = func(dir string, args ...string) ([]byte, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
