package sitedoctor

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	phpkg "github.com/geodro/lerd/internal/php"
)

// projectConfigFile is the per-project config every check here reads.
const projectConfigFile = ".lerd.yaml"

// checkProjectConfig validates the project's .lerd.yaml: the PHP version, the
// workers, the services, the container block and the commands. It is what `lerd
// check` used to be, folded in so one report answers whether the site is healthy
// instead of the user running a second command to find out.
func checkProjectConfig(path string, fw *config.Framework) (Check, bool) {
	if !fileExists(filepath.Join(path, projectConfigFile)) {
		return Check{}, false
	}
	problems, warnings := ValidateProjectConfig(path, fw)
	switch {
	case len(problems) > 0:
		return Check{Name: "project_config", Status: StatusFail, Detail: joinProblems(problems, warnings)}, true
	case len(warnings) > 0:
		return Check{Name: "project_config", Status: StatusWarn, Detail: joinProblems(nil, warnings)}, true
	default:
		return Check{Name: "project_config", Status: StatusOK}, true
	}
}

func joinProblems(problems, warnings []string) string {
	all := append(append([]string{}, problems...), warnings...)
	return strings.Join(all, " · ")
}

// ValidateProjectConfig reads the project's .lerd.yaml and returns what is wrong
// with it: problems that stop the site from coming up, and warnings that only
// degrade it. fw is the framework already resolved for the project, or nil to
// resolve it from the file. Both results are empty for a valid file.
func ValidateProjectConfig(path string, fw *config.Framework) (problems, warnings []string) {
	cfg, err := config.LoadProjectConfig(path)
	if err != nil {
		return []string{fmt.Sprintf("%s is not valid YAML: %v", projectConfigFile, err)}, nil
	}

	problem := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }
	warn := func(format string, a ...any) { warnings = append(warnings, fmt.Sprintf(format, a...)) }

	if cfg.PHPVersion != "" {
		switch {
		case !validPHPVersion(cfg.PHPVersion):
			problem("php_version: %q is not a MAJOR.MINOR version", cfg.PHPVersion)
		case !phpkg.IsInstalled(cfg.PHPVersion):
			warn("php_version: %s is not installed, run lerd php:install %s", cfg.PHPVersion, cfg.PHPVersion)
		}
	}
	if cfg.RequestTimeout < 0 {
		problem("request_timeout: %d must be a positive number of seconds", cfg.RequestTimeout)
	}
	if cfg.Framework != "" && cfg.FrameworkDef == nil {
		if _, ok := config.GetFramework(cfg.Framework); !ok {
			warn("framework: %q is not a known or user-defined framework", cfg.Framework)
		}
	}

	p, w := validateWorkers(path, cfg, fw)
	problems, warnings = append(problems, p...), append(warnings, w...)
	problems = append(problems, validateServices(cfg)...)
	p, w = validateContainer(path, cfg)
	problems, warnings = append(problems, p...), append(warnings, w...)
	p, w = validateCommands(cfg)
	problems, warnings = append(problems, p...), append(warnings, w...)

	// Sorted, so two runs on an unchanged file read the same.
	for _, name := range slices.Sorted(maps.Keys(cfg.CustomWorkers)) {
		if cfg.CustomWorkers[name].Command == "" {
			problems = append(problems, fmt.Sprintf("custom_worker.%s: command is required", name))
		}
	}
	return problems, warnings
}

func validateWorkers(path string, cfg *config.ProjectConfig, fw *config.Framework) (problems, warnings []string) {
	if len(cfg.Workers) == 0 {
		return nil, nil
	}
	// A custom-container site runs its workers from custom_workers, so the
	// framework's worker definitions never apply to it.
	if cfg.Container != nil {
		for _, w := range cfg.Workers {
			if _, ok := cfg.CustomWorkers[w]; !ok && !config.IsBuiltinWorker(w) {
				problems = append(problems, fmt.Sprintf("worker: %q is not defined in custom_workers", w))
			}
		}
		return problems, nil
	}

	if fw == nil {
		fw, _ = config.GetFrameworkForDir(cfg.Framework, path)
	}
	hasQueue, hasHorizon := false, false
	for _, w := range cfg.Workers {
		hasQueue = hasQueue || w == "queue"
		hasHorizon = hasHorizon || w == "horizon"
		// A worker the project defines itself is its own definition, whether or
		// not the site runs a custom container.
		if _, ok := cfg.CustomWorkers[w]; ok || config.IsBuiltinWorker(w) {
			continue
		}
		if fw == nil || fw.Workers == nil {
			warnings = append(warnings, fmt.Sprintf("worker: %q has no worker definitions to match", w))
			continue
		}
		wDef, ok := fw.Workers[w]
		if !ok {
			problems = append(problems, fmt.Sprintf("worker: %q is not defined for framework %s", w, cfg.Framework))
			continue
		}
		if wDef.Check != nil && !config.MatchesRule(path, *wDef.Check) {
			warnings = append(warnings, fmt.Sprintf("worker: %s prerequisite not met (check rule failed)", w))
		}
	}
	if hasQueue && hasHorizon {
		warnings = append(warnings, "workers: both queue and horizon are listed, horizon manages queues so the queue worker is skipped")
	}
	if hasQueue && siteHasHorizon(path) {
		warnings = append(warnings, "workers: queue is listed but laravel/horizon is installed, horizon is started instead")
	}
	return problems, warnings
}

// validateServices judges how the services are declared, not whether they are
// installed: the required_services check already reports a missing one, and does
// it with a fix attached.
func validateServices(cfg *config.ProjectConfig) (problems []string) {
	for _, svc := range cfg.Services {
		switch {
		case svc.Custom != nil:
			if svc.Custom.Image == "" {
				problems = append(problems, fmt.Sprintf("service %q: the inline definition has no image", svc.Name))
			}
		case svc.Preset != "":
			if _, err := config.LoadPreset(svc.Preset); err != nil {
				problems = append(problems, fmt.Sprintf("service %q: unknown preset %q", svc.Name, svc.Preset))
			}
		}
	}
	return problems
}

func validateContainer(path string, cfg *config.ProjectConfig) (problems, warnings []string) {
	if cfg.Container == nil {
		return nil, nil
	}
	if cfg.Container.Port <= 0 || cfg.Container.Port > 65535 {
		problems = append(problems, "container.port: required, and must be 1-65535")
	}
	cf := cfg.Container.Containerfile
	if cf == "" {
		cf = "Containerfile.lerd"
	}
	if !fileExists(filepath.Join(path, cf)) {
		warnings = append(warnings, fmt.Sprintf("container.containerfile: %s not found, lerd link will fail", cf))
	}
	if bc := cfg.Container.BuildContext; bc != "" {
		if _, err := os.Stat(filepath.Join(path, bc)); err != nil {
			warnings = append(warnings, fmt.Sprintf("container.build_context: %s not found", bc))
		}
	}
	return problems, warnings
}

func validateCommands(cfg *config.ProjectConfig) (problems, warnings []string) {
	seen := map[string]bool{}
	for i, c := range cfg.Commands {
		if c.Name == "" {
			problems = append(problems, fmt.Sprintf("commands[%d]: name is required", i))
			continue
		}
		if seen[c.Name] {
			problems = append(problems, fmt.Sprintf("command %q: duplicate name", c.Name))
			continue
		}
		seen[c.Name] = true
		if c.Disabled {
			continue
		}
		if c.Command == "" {
			problems = append(problems, fmt.Sprintf("command %q: command is required (or set disabled: true)", c.Name))
			continue
		}
		if c.Output != "" && !slices.Contains(config.ValidCommandOutputs, c.Output) {
			problems = append(problems, fmt.Sprintf("command %q: output %q is invalid (expected: %v)", c.Name, c.Output, config.ValidCommandOutputs))
			continue
		}
		if c.Label == "" {
			warnings = append(warnings, fmt.Sprintf("command %q: label is empty, the UI falls back to the name", c.Name))
		}
		if c.Icon != "" && !slices.Contains(config.KnownCommandIcons, c.Icon) {
			warnings = append(warnings, fmt.Sprintf("command %q: icon %q is not in the known set, the UI falls back to a generic one", c.Name, c.Icon))
		}
	}
	return problems, warnings
}

// validPHPVersion reports whether s looks like MAJOR.MINOR, rejecting "8,5" and
// plain words the way the init prompt does.
func validPHPVersion(s string) bool {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		if p == "" || strings.TrimLeft(p, "0123456789") != "" {
			return false
		}
	}
	return true
}

// siteHasHorizon reports whether composer.json lists laravel/horizon.
func siteHasHorizon(path string) bool {
	data, err := os.ReadFile(filepath.Join(path, "composer.json"))
	return err == nil && strings.Contains(string(data), `"laravel/horizon"`)
}
