package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
)

// resolveSetupServices brings up the services the selected worker steps need
// before any step runs, since a step's output is captured and cannot prompt.
// auto installs without asking; a declined service deselects its workers.
func resolveSetupServices(steps []setupStep, selected map[string]bool, auto bool,
	running func(string) bool, confirm func(string) bool, ensure func(string) error) {
	seen := map[string]bool{}
	for _, s := range steps {
		name := s.service
		if name == "" || !selected[s.label] || seen[name] || running(name) {
			continue
		}
		seen[name] = true
		if !auto && !confirm(fmt.Sprintf("Workers need the %s service, which is not running. Install and start it?", name)) {
			for _, w := range steps {
				if w.service == name && selected[w.label] {
					delete(selected, w.label)
					feedback.Note(fmt.Sprintf("skipping %s: needs %s", w.label, name))
				}
			}
			continue
		}
		step := feedback.Start("starting " + name)
		if err := ensure(name); err != nil {
			step.Fail(err)
			continue
		}
		step.OK("")
	}
}

func serviceRunning(name string) bool {
	running, _ := podman.ContainerRunning("lerd-" + name)
	return running
}

// ensureSetupService installs a store preset that isn't installed yet, then
// starts it the way `lerd service start` does, which also lifts a pause the
// user set earlier; a plain ensure would be stopped again by that pause.
func ensureSetupService(name string) error {
	if !serviceops.IsBuiltin(name) && !serviceops.ServiceInstalled(name) {
		if _, err := serviceops.InstallPresetByName(name, ""); err != nil {
			return err
		}
	}
	return serviceops.StartService(name)
}
