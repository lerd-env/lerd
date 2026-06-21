package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/spf13/cobra"
)

// check-report markers, coloured once via the shared palette (plain when piped
// / NO_COLOR). ckOK/ckWarn/ckFail are printf wrappers so go vet keeps verifying
// the format strings at every call site.
var (
	ckOKGlyph   = feedback.Green("✓")
	ckWarnGlyph = feedback.Amber("⚠")
	ckFailGlyph = feedback.Red("✗")
)

func ckOK(format string, a ...any)   { fmt.Printf("  %s %s", ckOKGlyph, fmt.Sprintf(format, a...)) }
func ckWarn(format string, a ...any) { fmt.Printf("  %s %s", ckWarnGlyph, fmt.Sprintf(format, a...)) }
func ckFail(format string, a ...any) { fmt.Printf("  %s %s", ckFailGlyph, fmt.Sprintf(format, a...)) }

// NewCheckCmd returns the check command.
func NewCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate .lerd.yaml — PHP version, services, workers, container config, custom_workers, and db",
		RunE:  runCheck,
	}
}

func runCheck(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	path := filepath.Join(cwd, ".lerd.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("no .lerd.yaml found in %s — run lerd init to create one", cwd)
	}

	cfg, err := config.LoadProjectConfig(cwd)
	if err != nil {
		ckFail(".lerd.yaml has invalid YAML syntax\n")
		fmt.Printf("        %v\n", err)
		return fmt.Errorf("validation failed")
	}

	warnings := 0
	errors := 0

	// PHP version
	if cfg.PHPVersion != "" {
		if err := validatePHPVersion(cfg.PHPVersion); err != nil {
			ckFail("php_version: %s — %v\n", cfg.PHPVersion, err)
			errors++
		} else if !phpPkg.IsInstalled(cfg.PHPVersion) {
			ckWarn("php_version: %s is not installed — run lerd php:install %s\n", cfg.PHPVersion, cfg.PHPVersion)
			warnings++
		} else {
			ckOK("php_version: %s\n", cfg.PHPVersion)
		}
	}

	// Node version
	if cfg.NodeVersion != "" {
		ckOK("node_version: %s\n", cfg.NodeVersion)
	}

	// Request timeout
	if cfg.RequestTimeout != 0 {
		if cfg.RequestTimeout < 0 {
			ckFail("request_timeout: %d — must be a positive number of seconds\n", cfg.RequestTimeout)
			errors++
		} else {
			ckOK("request_timeout: %ds\n", cfg.RequestTimeout)
		}
	}

	// Framework
	if cfg.Framework != "" {
		if cfg.FrameworkDef != nil {
			ckOK("framework: %s (inline definition)\n", cfg.Framework)
		} else if _, ok := config.GetFramework(cfg.Framework); ok {
			ckOK("framework: %s\n", cfg.Framework)
		} else {
			ckWarn("framework: %q is not a known or user-defined framework\n", cfg.Framework)
			warnings++
		}
	}

	// Secured
	if cfg.Secured {
		ckOK("secured: true\n")
	}

	// Domains
	if len(cfg.Domains) > 0 {
		ckOK("domains: %v\n", cfg.Domains)
	}

	// Workers
	if len(cfg.Workers) > 0 {
		if cfg.Container != nil {
			// Custom container site: workers must be defined in custom_workers.
			for _, w := range cfg.Workers {
				if _, ok := cfg.CustomWorkers[w]; ok {
					ckOK("worker: %s\n", w)
				} else {
					ckFail("worker: %q is not defined in custom_workers\n", w)
					errors++
				}
			}
		} else {
			fwName := cfg.Framework
			fw, hasFw := config.GetFrameworkForDir(fwName, cwd)

			hasQueue := false
			hasHorizon := false
			for _, w := range cfg.Workers {
				if w == "queue" {
					hasQueue = true
				}
				if w == "horizon" {
					hasHorizon = true
				}

				if !hasFw || fw.Workers == nil {
					if fwName != "" {
						ckWarn("worker: %q — framework %s has no worker definitions\n", w, fwName)
						warnings++
					} else {
						ckWarn("worker: %q — no framework detected\n", w)
						warnings++
					}
					continue
				}
				wDef, ok := fw.Workers[w]
				if !ok {
					ckFail("worker: %q is not defined for framework %s\n", w, fwName)
					errors++
					continue
				}
				if wDef.Check != nil && !config.MatchesRule(cwd, *wDef.Check) {
					ckWarn("worker: %s — prerequisite not met (check rule failed)\n", w)
					warnings++
				} else {
					ckOK("worker: %s\n", w)
				}
			}

			if hasQueue && hasHorizon {
				ckWarn("workers: both queue and horizon are listed — horizon manages queues, queue worker will be skipped\n")
				warnings++
			}
			if hasQueue && SiteHasHorizon(cwd) {
				ckWarn("workers: queue is listed but laravel/horizon is installed — horizon will be started instead\n")
				warnings++
			}
		}
	}

	// Services
	for _, svc := range cfg.Services {
		if svc.Custom != nil {
			// Inline definition — check required fields.
			if svc.Custom.Image == "" {
				ckFail("service %q: inline definition is missing required \"image\" field\n", svc.Name)
				errors++
			} else {
				ckOK("service: %s (inline, image: %s)\n", svc.Name, svc.Custom.Image)
			}
			continue
		}

		if svc.Preset != "" {
			// Preset reference — verify the preset exists in the catalog, then
			// check whether it has been installed on this machine.
			if _, err := config.LoadPreset(svc.Preset); err != nil {
				ckFail("service %q: unknown preset %q\n", svc.Name, svc.Preset)
				errors++
			} else if _, err := config.LoadCustomService(svc.Name); err != nil {
				ckWarn("service %s: preset %q not installed — run: lerd service preset install %s\n", svc.Name, svc.Preset, svc.Preset)
				warnings++
			} else {
				ckOK("service: %s (preset: %s)\n", svc.Name, svc.Preset)
			}
			continue
		}

		if isKnownService(svc.Name) {
			ckOK("service: %s\n", svc.Name)
			continue
		}

		if serviceops.ServiceInstalled(svc.Name) {
			ckOK("service: %s (custom)\n", svc.Name)
		} else {
			ckFail("service %q: not installed — run `lerd service preset install %s` (if it's a bundled preset) or `lerd service add --name %s ...`\n",
				svc.Name, svc.Name, svc.Name)
			errors++
		}
	}

	// Container
	if cfg.Container != nil {
		if cfg.Container.Port <= 0 || cfg.Container.Port > 65535 {
			ckFail("container.port: required and must be 1–65535\n")
			errors++
		} else {
			ckOK("container.port: %d\n", cfg.Container.Port)
		}
		cfPath := cfg.Container.Containerfile
		if cfPath == "" {
			cfPath = "Containerfile.lerd"
		}
		if _, err := os.Stat(filepath.Join(cwd, cfPath)); os.IsNotExist(err) {
			ckWarn("container.containerfile: %s not found — lerd link will fail\n", cfPath)
			warnings++
		} else {
			ckOK("container.containerfile: %s\n", cfPath)
		}
		if cfg.Container.BuildContext != "" {
			if _, err := os.Stat(filepath.Join(cwd, cfg.Container.BuildContext)); os.IsNotExist(err) {
				ckWarn("container.build_context: %s not found\n", cfg.Container.BuildContext)
				warnings++
			} else {
				ckOK("container.build_context: %s\n", cfg.Container.BuildContext)
			}
		}
		if cfg.Container.SSL {
			ckOK("container.ssl: true (nginx will proxy_pass via HTTPS with ssl_verify off)\n")
		}
	}

	// custom_workers
	for name, w := range cfg.CustomWorkers {
		if w.Command == "" {
			ckFail("custom_worker.%s: command is required\n", name)
			errors++
		} else {
			ckOK("custom_worker.%s\n", name)
		}
	}

	// commands
	seenCmdNames := map[string]bool{}
	for i, c := range cfg.Commands {
		if c.Name == "" {
			ckFail("commands[%d]: name is required\n", i)
			errors++
			continue
		}
		if seenCmdNames[c.Name] {
			ckFail("command %q: duplicate name\n", c.Name)
			errors++
			continue
		}
		seenCmdNames[c.Name] = true
		if c.Disabled {
			ckOK("command.%s (disabled)\n", c.Name)
			continue
		}
		if c.Command == "" {
			ckFail("command %q: command is required (or set disabled: true)\n", c.Name)
			errors++
			continue
		}
		if c.Label == "" {
			ckWarn("command %q: label is empty, the UI will fall back to the name\n", c.Name)
			warnings++
		}
		if c.Output != "" && !slices.Contains(config.ValidCommandOutputs, c.Output) {
			ckFail("command %q: output %q is invalid (expected: %v)\n", c.Name, c.Output, config.ValidCommandOutputs)
			errors++
			continue
		}
		if c.Icon != "" && !slices.Contains(config.KnownCommandIcons, c.Icon) {
			ckWarn("command %q: icon %q is not in the known set, UI will fall back to a generic icon\n", c.Name, c.Icon)
			warnings++
		}
		ckOK("command.%s\n", c.Name)
	}

	// db
	if cfg.DB.Service != "" {
		if isKnownService(cfg.DB.Service) {
			ckOK("db.service: %s\n", cfg.DB.Service)
		} else if serviceops.ServiceInstalled(cfg.DB.Service) {
			ckOK("db.service: %s (custom)\n", cfg.DB.Service)
		} else {
			ckFail("db.service: %q is not a known service\n", cfg.DB.Service)
			errors++
		}
	}

	// Summary
	fmt.Println()
	if errors > 0 {
		fmt.Printf("  %d error(s), %d warning(s)\n", errors, warnings)
		return fmt.Errorf("validation failed")
	}
	if warnings > 0 {
		fmt.Printf("  %d warning(s), no errors\n", warnings)
	} else {
		fmt.Printf("  .lerd.yaml is valid\n")
	}
	return nil
}
