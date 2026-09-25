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
// the same two steps the setup wizard runs after its questions.
func addSiteService(site *config.Site, name string) error {
	e := siteinfo.Enrich(*site, siteinfo.EnrichFramework|siteinfo.EnrichServices)
	if !slices.ContainsFunc(e.SuggestedServices, func(sg config.ServiceSuggestion) bool { return sg.Name == name }) {
		return fmt.Errorf("%q is not a service suggested for %s", name, site.Name)
	}
	svc := config.ProjectService{Name: name}
	if !config.IsDefaultPreset(name) {
		svc.Preset = name
	}
	if err := config.AddProjectServices(site.Path, []config.ProjectService{svc}); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	for _, args := range [][]string{{"link", "--yes"}, {"env"}} {
		cmd := exec.Command(self, args...)
		cmd.Dir = site.Path
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("lerd %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(lastLines(string(out), 5)))
		}
	}
	return nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
