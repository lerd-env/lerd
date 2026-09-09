package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/linker"
	nodeDet "github.com/geodro/lerd/internal/node"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewInitCmd returns the init command.
func NewInitCmd() *cobra.Command {
	var fresh bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a project: run the setup wizard and save .lerd.yaml",
		Long: `Run the setup wizard to configure PHP version, HTTPS, and required services,
then save the answers to .lerd.yaml in the current directory.

If .lerd.yaml already exists the wizard is skipped and the saved configuration
is applied directly. Use --fresh to re-run the wizard with existing values as
defaults.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInit(fresh)
		},
	}
	cmd.Flags().BoolVar(&fresh, "fresh", false, "Re-run the wizard even if .lerd.yaml already exists")
	return cmd
}

func runInit(fresh bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	lerdYAMLPath := filepath.Join(cwd, ".lerd.yaml")
	_, statErr := os.Stat(lerdYAMLPath)
	hasExisting := statErr == nil

	feedback.Begin()

	if initShouldRunWizard(hasExisting, fresh) {
		// The wizard is a full-screen form and cannot open one without a
		// terminal. Entering it anyway surfaced the form library's own
		// "error opening TTY", which names nothing the user can act on.
		if !initInteractiveFn() {
			return errors.New("lerd init runs a wizard and needs a terminal\n       run 'lerd link' to register this project without one, or write .lerd.yaml yourself")
		}
		existing, err := config.LoadProjectConfig(cwd)
		if err != nil {
			return err
		}
		existing = applyImportSeed(cwd, existing)
		cfg, err := runWizard(cwd, existing)
		if err != nil {
			return err
		}
		write := feedback.Start("writing .lerd.yaml")
		if err := config.SaveProjectConfig(cwd, cfg); err != nil {
			write.Fail(err)
			return fmt.Errorf("saving .lerd.yaml: %w", err)
		}
		write.OK("")
		// The wizard already had the user choose the dev command, so the link
		// it triggers below shouldn't prompt to confirm that same command again.
		hostProxyPreApproved = true
		defer func() { hostProxyPreApproved = false }()
	}

	if err := applyProjectConfig(cwd); err != nil {
		return err
	}

	if isInteractive() {
		if feedback.Confirm("Run lerd setup?", true) {
			if err := runSetup(false, false); err != nil {
				feedback.Warn("setup: %v", err)
			}
		}
	}

	return nil
}

// initShouldRunWizard reports whether runInit runs the configuration wizard
// rather than applying an existing .lerd.yaml directly. It runs when no config
// file is present, or when fresh forces it (the `lerd link` route passes fresh
// for an absent-or-empty config, and `lerd init --fresh` for an explicit redo).
func initShouldRunWizard(hasExisting, fresh bool) bool {
	return !hasExisting || fresh
}

// initInteractiveFn is a seam so the no-terminal refusal can be tested.
var initInteractiveFn = isInteractive

// nodeVersionDefault prefills the Node field the way the PHP one above it is
// prefilled: a version already saved in .lerd.yaml wins, otherwise the version
// the project resolves to on its own.
func nodeVersionDefault(saved, resolved string) string {
	if saved != "" {
		return saved
	}
	return resolved
}

// nodeVersionDescription says what clearing the Node field does. An answer there
// writes a node_version pin that outranks .nvmrc, .node-version, engines.node
// and the machine default, and keeps outranking them, so an empty field is not
// Node being turned off, it is the project going on resolving its own version.
func nodeVersionDescription(source string) string {
	if source == "" {
		return "Clear to leave the version unpinned"
	}
	return fmt.Sprintf("Clear to follow %s instead of pinning", source)
}

func runWizard(cwd string, defaults *config.ProjectConfig) (*config.ProjectConfig, error) {
	gcfg, err := config.LoadGlobal()
	if err != nil {
		return nil, err
	}

	// Decide whether to offer the custom container wizard.
	// If the existing config already has a container section (re-running
	// --fresh), go straight to it. Otherwise check the project: no
	// composer.json + no detected framework suggests a non-PHP project.
	framework, hasFramework := resolveFramework(cwd)
	hasComposer := fileExists(filepath.Join(cwd, "composer.json"))
	hasContainerfile := podman.HasContainerfile(cwd)
	alreadyCustom := defaults.Container != nil
	alreadyProxy := defaults.Proxy != nil

	if alreadyProxy {
		return runHostProxyWizard(cwd, defaults, gcfg)
	}
	if alreadyCustom {
		return runCustomContainerWizard(cwd, defaults, gcfg)
	}
	if !hasFramework && !hasComposer && hasContainerfile {
		return runCustomContainerWizard(cwd, defaults, gcfg)
	}
	if !hasFramework && !hasComposer {
		// Non-PHP project. Offer the same language-aware choice to every runtime,
		// not just Node: run the dev server on the host (proxy) or build a custom
		// container. When the project's runtime is recognised we say so and drop
		// the plain-PHP option; an unknown/empty directory keeps it as a fallback
		// (declining both non-PHP paths sets the project up as a PHP site).
		const proxyChoice = "Dev server (proxy to a host port)"
		const customChoice = "Custom container (Containerfile.lerd)"
		const phpChoice = "Plain PHP site"

		rt, knownRuntime := detectProjectRuntime(cwd)
		title := "No PHP project detected. How should lerd run it?"
		options := []string{proxyChoice, customChoice, phpChoice}
		if knownRuntime {
			title = fmt.Sprintf("This looks like a %s project. How should lerd run it?", rt.label)
			options = []string{proxyChoice, customChoice}
		}

		choice := proxyChoice
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(huh.NewOptions(options...)...).
				Value(&choice),
		)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
			return nil, err
		}
		switch choice {
		case proxyChoice:
			return runHostProxyWizard(cwd, defaults, gcfg)
		case customChoice:
			return runCustomContainerWizard(cwd, defaults, gcfg)
		}
		// phpChoice falls through to the PHP wizard below.
	}

	// Resolved before the seeding below, which fills defaults from the registry
	// and would leave an unconfigured project looking like a configured one.
	secured, httpsAvailable := resolveSecuredDefault(cwd, defaults, gcfg)

	// Seed defaults from the site registry when no saved config exists yet,
	// so already-set PHP version and HTTPS state are reflected on first run.
	if defaults.PHPVersion == "" && !defaults.Secured {
		if site, err := config.FindSiteByPath(cwd); err == nil {
			if defaults.PHPVersion == "" {
				defaults.PHPVersion = site.PHPVersion
			}
			if !defaults.Secured {
				defaults.Secured = site.Secured
			}
		}
	}

	phpDefault := wizardPHPDefault(cwd, defaults, gcfg, framework)

	// Database is picked as a single choice (sqlite | mysql family member |
	// postgres family member), while other services are a multi-select. This
	// mirrors the runtime prompt in `lerd env` and prevents users from
	// accidentally selecting both mysql and postgres for the same project.
	// Multi-version mysql/postgres alternates installed via presets show up as
	// extra Database options instead of polluting the Services list.
	dbFramework, _ := config.GetFrameworkForDir(framework, cwd)
	dbOptions, dbNameSet := buildDatabaseOptions(dbFramework)
	nonDBServiceOptions := nonDatabaseServiceNames(dbNameSet)
	dbChoice, nonDBSelected := wizardServiceDefaults(cwd, defaults, dbNameSet)

	phpVersion := phpDefault
	nodeVersion := defaults.NodeVersion

	// FrankenPHP detection. If the project has signals we offer it as a
	// choice in the wizard; default to whatever the existing config says.
	frankenHints := config.DetectFrankenPHPHints(cwd)
	useFrankenPHP := defaults.Runtime == "frankenphp"
	useFrankenPHPWorker := defaults.RuntimeWorker

	selectedWorkers := defaults.Workers
	if len(selectedWorkers) == 0 {
		selectedWorkers = []string{}
	}

	// If there are custom workers from the existing config, let the user
	// choose which to keep before the workers step.
	var customWorkerNames []string
	var keepCustomWorkers []string
	if len(defaults.CustomWorkers) > 0 {
		for name := range defaults.CustomWorkers {
			customWorkerNames = append(customWorkerNames, name)
		}
		sort.Strings(customWorkerNames)
		keepCustomWorkers = make([]string, len(customWorkerNames))
		copy(keepCustomWorkers, customWorkerNames)
	}

	firstGroupFields := []huh.Field{
		huh.NewInput().
			Title("PHP version").
			Value(&phpVersion).
			Validate(func(s string) error {
				if s == "" {
					return nil
				}
				return validatePHPVersion(s)
			}),
	}
	if lerdManagesNode() {
		resolved, source := nodeDet.UnpinnedVersion(cwd)
		nodeVersion = nodeVersionDefault(nodeVersion, resolved)
		firstGroupFields = append(firstGroupFields,
			huh.NewInput().
				Title("Node version").
				Description(nodeVersionDescription(source)).
				Value(&nodeVersion),
		)
	}
	firstGroupFields = appendHTTPSField(firstGroupFields, httpsAvailable, &secured)
	firstGroupFields = append(firstGroupFields,
		huh.NewSelect[string]().
			Title("Database").
			Options(dbOptions...).
			Value(&dbChoice),
		newMultiSelect("Services", "", nonDBServiceOptions, &nonDBSelected),
	)

	formGroups := []*huh.Group{huh.NewGroup(firstGroupFields...)}

	if len(frankenHints) > 0 || useFrankenPHP {
		reason := "Detected FrankenPHP signals in this project"
		if len(frankenHints) > 0 {
			reason = frankenHints[0].Reason
		}
		formGroups = append(formGroups, huh.NewGroup(
			huh.NewConfirm().
				Title("Use FrankenPHP runtime?").
				Description(reason).
				Value(&useFrankenPHP),
			huh.NewConfirm().
				Title("Enable worker mode?").
				Description("Keeps PHP resident, ~10-50x faster requests, trades some dev ergonomics").
				Value(&useFrankenPHPWorker),
		))
	}

	if len(customWorkerNames) > 0 {
		formGroups = append(formGroups, huh.NewGroup(
			newMultiSelect("Custom workers", "Deselect to remove from .lerd.yaml", customWorkerNames, &keepCustomWorkers),
		))
	}

	if err := huh.NewForm(formGroups...).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
		return nil, err
	}

	// Build the set of kept custom workers.
	keptSet := make(map[string]bool, len(keepCustomWorkers))
	for _, name := range keepCustomWorkers {
		keptSet[name] = true
	}

	// Removed custom workers are excluded, and their conflict rules no longer
	// apply, so a worker one of them suppressed becomes available again.
	removedCustom := map[string]bool{}
	if fw, ok := config.GetFrameworkForDir(framework, cwd); ok {
		for name := range fw.Workers {
			if defaults.CustomWorkers[name].Command != "" && !keptSet[name] {
				removedCustom[name] = true
			}
		}
	}
	workerOptions := frameworkWorkerOptions(cwd, framework, removedCustom)
	selectedWorkers = keepAvailable(selectedWorkers, workerOptions)

	if len(workerOptions) > 0 {
		workerGroups := []*huh.Group{
			huh.NewGroup(
				newMultiSelect("Workers", "Auto-start when linking", workerOptions, &selectedWorkers),
			),
		}
		if err := huh.NewForm(workerGroups...).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
			return nil, err
		}
	}

	selectedServices := persistedServices(dbChoice, nonDBSelected)

	// Only embed the framework definition in .lerd.yaml for user-defined
	// frameworks that aren't available from the store. Built-in (laravel) and
	// store-installed frameworks can be fetched on any machine.
	var frameworkDef *config.Framework
	if framework != "" {
		info := config.GetFrameworkSource(framework)
		if info == config.SourceUser {
			if fw, ok := config.GetFramework(framework); ok {
				frameworkDef = fw
			}
		}
	}

	services := buildProjectServices(selectedServices, defaults)

	// Resolve framework version from the definition that was used.
	frameworkVersion := ""
	if frameworkDef != nil && frameworkDef.Version != "" {
		frameworkVersion = frameworkDef.Version
	} else if fw, ok := config.GetFrameworkForDir(framework, cwd); ok && fw.Version != "" {
		frameworkVersion = fw.Version
	}

	// Filter custom workers to only those the user chose to keep.
	var filteredCustomWorkers map[string]config.FrameworkWorker
	if len(keepCustomWorkers) > 0 {
		filteredCustomWorkers = make(map[string]config.FrameworkWorker, len(keepCustomWorkers))
		for _, name := range keepCustomWorkers {
			if w, ok := defaults.CustomWorkers[name]; ok {
				filteredCustomWorkers[name] = w
			}
		}
	}

	runtime := ""
	runtimeWorker := false
	if useFrankenPHP {
		runtime = "frankenphp"
		runtimeWorker = useFrankenPHPWorker
	}

	return &config.ProjectConfig{
		PHPVersion:       phpVersion,
		NodeVersion:      nodeVersion,
		Framework:        framework,
		FrameworkVersion: frameworkVersion,
		FrameworkDef:     frameworkDef,
		PublicDir:        defaults.PublicDir,
		Secured:          persistedSecured(secured, httpsAvailable, defaults.Secured),
		Services:         services,
		Workers:          selectedWorkers,
		CustomWorkers:    filteredCustomWorkers,
		AppURL:           defaults.AppURL,
		Domains:          defaults.Domains,
		Runtime:          runtime,
		RuntimeWorker:    runtimeWorker,
	}, nil
}

// runCustomContainerWizard runs the init wizard for custom container projects.
// It collects the container port, containerfile path, HTTPS, services, and
// custom workers, then returns a ProjectConfig with the container section.
func runCustomContainerWizard(cwd string, defaults *config.ProjectConfig, gcfg *config.GlobalConfig) (*config.ProjectConfig, error) {
	portStr := "3000"
	containerfile := "Containerfile.lerd"
	secured, httpsAvailable := resolveSecuredDefault(cwd, defaults, gcfg)

	if defaults.Container != nil {
		if defaults.Container.Port > 0 {
			portStr = fmt.Sprintf("%d", defaults.Container.Port)
		}
		if defaults.Container.Containerfile != "" {
			containerfile = defaults.Container.Containerfile
		}
	}

	containerFields := []huh.Field{
		huh.NewInput().
			Title("Container port").
			Description("Port the app listens on inside the container").
			Value(&portStr).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("port is required")
				}
				for _, c := range s {
					if c < '0' || c > '9' {
						return fmt.Errorf("port must be a number")
					}
				}
				return nil
			}),
		huh.NewInput().
			Title("Containerfile").
			Description("Path relative to project root").
			Value(&containerfile),
	}
	containerFields = appendHTTPSField(containerFields, httpsAvailable, &secured)
	if err := huh.NewForm(huh.NewGroup(containerFields...)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
		return nil, err
	}

	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	// Offer to scaffold the Containerfile if it isn't there yet, so picking
	// "custom container" on a project without one isn't a dead end.
	maybeCreateContainerfile(cwd, containerfile, port)

	// Services: same flow as the PHP wizard but without the database select
	// since custom containers manage their own database connections.
	serviceOptions := nonDatabaseServiceOptions()

	serviceDefaults := defaults.ServiceNames()
	var selectedServices []string
	copy(selectedServices, serviceDefaults)
	selectedServices = serviceDefaults

	if len(serviceOptions) > 0 {
		if err := huh.NewForm(huh.NewGroup(
			newMultiSelect("Services", "", serviceOptions, &selectedServices),
		)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
			return nil, err
		}
	}

	// Custom workers from existing config.
	var customWorkerNames []string
	var keepCustomWorkers []string
	if len(defaults.CustomWorkers) > 0 {
		for name := range defaults.CustomWorkers {
			customWorkerNames = append(customWorkerNames, name)
		}
		sort.Strings(customWorkerNames)
		keepCustomWorkers = make([]string, len(customWorkerNames))
		copy(keepCustomWorkers, customWorkerNames)

		if err := huh.NewForm(huh.NewGroup(
			newMultiSelect("Custom workers", "Deselect to remove from .lerd.yaml", customWorkerNames, &keepCustomWorkers),
		)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
			return nil, err
		}
	}

	// Build services list.
	services := buildProjectServices(selectedServices, defaults)

	// Filter custom workers.
	var filteredCustomWorkers map[string]config.FrameworkWorker
	if len(keepCustomWorkers) > 0 {
		filteredCustomWorkers = make(map[string]config.FrameworkWorker, len(keepCustomWorkers))
		for _, name := range keepCustomWorkers {
			if w, ok := defaults.CustomWorkers[name]; ok {
				filteredCustomWorkers[name] = w
			}
		}
	}

	containerCfg := &config.ContainerConfig{
		Port: port,
	}
	if containerfile != "Containerfile.lerd" && containerfile != "" {
		containerCfg.Containerfile = containerfile
	}

	return &config.ProjectConfig{
		Secured:       persistedSecured(secured, httpsAvailable, defaults.Secured),
		Services:      services,
		CustomWorkers: filteredCustomWorkers,
		Container:     containerCfg,
		AppURL:        defaults.AppURL,
		Domains:       defaults.Domains,
		// This wizard never asks about Node, so a pin the project carries is
		// not its to drop.
		NodeVersion: defaults.NodeVersion,
	}, nil
}

// nonDatabaseServiceOptions returns selectable service names for the container
// and host-proxy wizards: all built-in presets plus custom services that
// integrate with .env. No database split — those wizards manage their own DB
// connections.
func nonDatabaseServiceOptions() []string {
	options := append([]string{}, knownServices()...)
	if customs, err := config.ListCustomServices(); err == nil {
		for _, svc := range customs {
			if len(svc.EnvVars) == 0 && svc.EnvDetect == nil {
				continue
			}
			options = append(options, svc.Name)
		}
	}
	return options
}

// buildProjectServices turns selected service names into ProjectService entries,
// resolving built-ins, presets, and inline custom services. Shared by the
// container and host-proxy wizards.
func buildProjectServices(selectedServices []string, defaults *config.ProjectConfig) []config.ProjectService {
	builtIn := make(map[string]bool)
	for _, s := range knownServices() {
		builtIn[s] = true
	}
	inlineByName := map[string]*config.CustomService{}
	for _, svc := range defaults.Services {
		if svc.Custom != nil {
			inlineByName[svc.Name] = svc.Custom
		}
	}

	services := make([]config.ProjectService, len(selectedServices))
	for i, name := range selectedServices {
		if builtIn[name] {
			services[i] = config.ProjectService{Name: name}
			continue
		}
		var loaded *config.CustomService
		if svc, err := config.LoadCustomService(name); err == nil {
			loaded = svc
		} else if existing := inlineByName[name]; existing != nil {
			loaded = existing
		}
		if loaded != nil && loaded.Preset != "" {
			services[i] = config.ProjectService{
				Name:          name,
				Preset:        loaded.Preset,
				PresetVersion: loaded.PresetVersion,
			}
			continue
		}
		// A bundled tool preset (e.g. phpmyadmin, pgadmin) selected but not yet
		// installed has no on-disk custom service; record it as a preset so link
		// installs it, rather than a bare name that resolves to nothing and warns.
		if loaded == nil && config.PresetExists(name) && !config.IsDefaultPreset(name) {
			services[i] = config.ProjectService{Name: name, Preset: name}
			continue
		}
		services[i] = config.ProjectService{Name: name, Custom: loaded}
	}
	return services
}

// runHostProxyWizard runs the init wizard for a host-proxy project (Node, or
// any runtime that serves on a host port): lerd supervises the dev command on
// the host and nginx proxies the domain to it. Command, port, and HTTPS are
// collected on a single screen (like the custom container wizard), followed by
// services.
func runHostProxyWizard(cwd string, defaults *config.ProjectConfig, gcfg *config.GlobalConfig) (*config.ProjectConfig, error) {
	manifest := readPackageManifest(cwd)
	devScripts := manifest.devScripts()

	// Default command: a saved one wins, else the first detected dev script,
	// else a runtime-appropriate guess. Blank is allowed (proxy-only mode).
	command := ""
	if defaults.Proxy != nil {
		command = defaults.Proxy.Command
	}
	if command == "" {
		if len(devScripts) > 0 {
			command = devScripts[0]
		} else {
			command = defaultDevCommand(cwd)
		}
	}
	commandDesc := "How lerd starts the app (lerd supervises and restarts it). Blank = run it yourself."
	if len(devScripts) > 0 {
		commandDesc = "Detected scripts: " + strings.Join(devScripts, ", ") + ". Blank = run it yourself."
	}

	// Port default: a saved one wins, else parse a --port from the command, else
	// auto-assign the next free dev-server port.
	port := 0
	if defaults.Proxy != nil && defaults.Proxy.Port > 0 {
		port = defaults.Proxy.Port
	}
	if port == 0 {
		if p := portFromCommand(command); p > 0 {
			port = p
		} else {
			siteName := ""
			if s, err := config.FindSiteByPath(cwd); err == nil {
				siteName = s.Name
			}
			port = allocateHostPort(defaultDevServerPort, siteName)
		}
	}
	portStr := strconv.Itoa(port)
	secured, httpsAvailable := resolveSecuredDefault(cwd, defaults, gcfg)

	proxyFields := []huh.Field{
		huh.NewInput().
			Title("Dev command").
			Description(commandDesc).
			Value(&command),
		huh.NewInput().
			Title("Port").
			Description("The port the dev server listens on (lerd injects PORT and proxies here)").
			Value(&portStr).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("port is required")
				}
				for _, c := range s {
					if c < '0' || c > '9' {
						return fmt.Errorf("port must be a number")
					}
				}
				return nil
			}),
	}
	proxyFields = appendHTTPSField(proxyFields, httpsAvailable, &secured)
	if err := huh.NewForm(huh.NewGroup(proxyFields...)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
		return nil, err
	}
	port = 0
	fmt.Sscanf(portStr, "%d", &port)
	// An explicit --port in the (possibly edited) command is the port the server
	// actually binds, so let it win over the port field, which only carried the
	// pre-form default derived from the default command.
	if p := portFromCommand(command); p > 0 {
		port = p
	}

	// Services multi-select (same flow as the custom container wizard).
	serviceOptions := nonDatabaseServiceOptions()
	selectedServices := defaults.ServiceNames()
	if len(serviceOptions) > 0 {
		if err := huh.NewForm(huh.NewGroup(
			newMultiSelect("Services", "", serviceOptions, &selectedServices),
		)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
			return nil, err
		}
	}

	// Vite rejects requests whose Host header isn't localhost/an IP (its
	// allowedHosts check). nginx proxies with the site domain as Host, so the
	// user has to allow it in vite.config or every request fails with a 403.
	// The raw Vite CLI also ignores the injected HOST env and binds loopback,
	// which the proxy container cannot reach, so it needs an explicit --host.
	if manifest.runsVite(command) {
		fmt.Println("\nNote: Vite blocks proxied requests by their Host header. Add your site")
		fmt.Println("domain to server.allowedHosts in vite.config (or set allowedHosts: true),")
		fmt.Println("or requests through the lerd proxy fail with \"host not allowed\".")
		fmt.Println("Vite also ignores the HOST env; add --host to the command so it binds")
		fmt.Println("all interfaces, otherwise the proxy can't reach it.")
	}

	return &config.ProjectConfig{
		Secured:     persistedSecured(secured, httpsAvailable, defaults.Secured),
		Services:    buildProjectServices(selectedServices, defaults),
		Proxy:       &config.ProxyConfig{Command: command, Port: port, SSL: false},
		AppURL:      defaults.AppURL,
		Domains:     defaults.Domains,
		NodeVersion: defaults.NodeVersion,
	}, nil
}

// dbFamilies is the set of service families considered databases by the init
// wizard. Members of these families show up in the Database select instead of
// the Services multi-select.
var dbFamilies = map[string]bool{
	"mysql":    true,
	"mariadb":  true,
	"postgres": true,
	"mongo":    true,
}

// dbFamilyOf returns the database family of svc, or empty when svc is not a
// database. Honours the explicit Family field first, then falls back to
// pattern inference for legacy installs that pre-date the field.
func dbFamilyOf(svc *config.CustomService) string {
	if family := config.FamilyOf(svc); dbFamilies[family] {
		return family
	}
	return ""
}

// dbFamilyLabels maps a family name to the human-friendly label prefix shown
// in the wizard's Database select.
var dbFamilyLabels = map[string]string{
	"mysql":    "MySQL",
	"mariadb":  "MariaDB",
	"postgres": "PostgreSQL",
	"mongo":    "MongoDB",
}

// formatDBOptionLabel returns "MySQL (lerd-mysql)" for the canonical family
// member or "MySQL 5.7 (lerd-mysql-5-7)" for a versioned alternate.
// persistedServices recombines the database pick and the non-database
// multi-select into the services list written to .lerd.yaml.
//
// SQLite is left out. It has no preset, no container and nothing to install, so
// an entry for it is one every surface then has to explain away, down to a card
// offering to install something that cannot exist. The project's own
// configuration already says it is on SQLite, and that is what lerd reads to
// answer which database a site uses.
func persistedServices(dbChoice string, nonDB []string) []string {
	out := make([]string, 0, len(nonDB)+1)
	if dbChoice != "" && dbChoice != "sqlite" {
		out = append(out, dbChoice)
	}
	return append(out, nonDB...)
}

// frameworkSupportsSQLite reports whether a framework can be wired to a file
// database, which the definition says with its own sqlite wiring. A project
// with no framework at all keeps the option: nothing has declared otherwise,
// and lerd should not decide for it.
func frameworkSupportsSQLite(fw *config.Framework) bool {
	if fw == nil || !fw.HasEnvConfig() {
		return true
	}
	return fw.Env.SQLite != nil
}

func formatDBOptionLabel(name string) string {
	family := name
	version := ""
	if inferred := config.FamilyOfName(name); inferred != "" {
		family = inferred
		if rest := strings.TrimPrefix(name, family); rest != "" && rest != name {
			version = strings.TrimPrefix(rest, "-")
			version = strings.ReplaceAll(version, "-", ".")
		}
	}
	label := dbFamilyLabels[family]
	if label == "" {
		label = strings.ToUpper(family[:1]) + family[1:]
	}
	if version != "" {
		label += " " + version
	}
	return fmt.Sprintf("%s (lerd-%s)", label, name)
}

// buildDatabaseOptions returns the Database select options and a set of every
// service name that lives in a database family (so the Services multi-select
// can filter them out). Always includes sqlite. Built-in mysql and postgres
// are always present; alternates and mongo show up only when installed.
func buildDatabaseOptions(fw *config.Framework) ([]huh.Option[string], map[string]bool) {
	nameSet := map[string]bool{}
	var options []huh.Option[string]

	// SQLite is offered where it means something: a framework declares the
	// service it can be wired through, and a project lerd recognises no
	// framework for keeps the option because there is no declaration to consult
	// and a file database is a reasonable answer for it. Offering it to a
	// framework that cannot use it is how a project ends up picking a database
	// its application will never open.
	if frameworkSupportsSQLite(fw) {
		nameSet["sqlite"] = true
		options = append(options, huh.NewOption("SQLite (no service)", "sqlite"))
	}

	for _, name := range []string{"mysql", "postgres"} {
		nameSet[name] = true
		options = append(options, huh.NewOption(formatDBOptionLabel(name), name))
	}

	if customs, err := config.ListCustomServices(); err == nil {
		var dbCustoms []*config.CustomService
		for _, svc := range customs {
			if dbFamilyOf(svc) != "" {
				dbCustoms = append(dbCustoms, svc)
			}
		}
		sort.Slice(dbCustoms, func(i, j int) bool { return dbCustoms[i].Name < dbCustoms[j].Name })
		for _, svc := range dbCustoms {
			nameSet[svc.Name] = true
			options = append(options, huh.NewOption(formatDBOptionLabel(svc.Name), svc.Name))
		}
	}

	return options, nameSet
}

// detectServicesFromDir inspects the project's env file and returns the list
// of services that appear to be in use. For frameworks that have explicit
// detection rules (e.g. wordpress, symfony), those rules are applied.
// For Laravel and unknown frameworks a set of standard heuristics is used.
func detectServicesFromDir(cwd string) []string {
	frameworkName, _ := resolveFramework(cwd)

	envFilePath := filepath.Join(cwd, ".env")
	envFormat := "dotenv"

	if fw, ok := config.GetFramework(frameworkName); ok {
		f, fmt := fw.Env.Resolve(cwd)
		envFilePath = filepath.Join(cwd, f)
		envFormat = fmt

		if len(fw.Env.Services) > 0 {
			return detectServicesFromRules(envExampleFallback(envFilePath), envFormat, fw.Env.Services)
		}
	}

	return detectServicesHeuristic(envExampleFallback(envFilePath), envFormat)
}

// detectDBConnection returns the lowercased DB_CONNECTION value from the
// project's env file, preferring .env over .env.example. Empty string when
// no env file exists or the key is unset.
func detectDBConnection(cwd string) string {
	frameworkName, _ := resolveFramework(cwd)

	envFilePath := filepath.Join(cwd, ".env")
	envFormat := "dotenv"

	if fw, ok := config.GetFramework(frameworkName); ok {
		f, fmtName := fw.Env.Resolve(cwd)
		envFilePath = filepath.Join(cwd, f)
		envFormat = fmtName
	}

	readKey := makeEnvReader(envExampleFallback(envFilePath), envFormat)
	return strings.ToLower(strings.TrimSpace(readKey("DB_CONNECTION")))
}

// envExampleFallback returns path if it exists, or path+".example" if that
// exists, otherwise path (callers already handle missing files gracefully).
func envExampleFallback(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if example := path + ".example"; fileExists(example) {
		return example
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// projectRuntime describes how lerd runs a non-PHP project of a given language.
// One entry drives all three consumers (detection, the host-proxy default dev
// command, and the custom-container starter), so adding a language is a single
// table entry rather than three switch/maps that must stay in sync.
type projectRuntime struct {
	label      string   // human name shown in the wizard
	manifests  []string // files whose presence identifies the runtime
	devCommand string   // default host-proxy dev command
	// container is the Containerfile body for this runtime. lerd bind-mounts the
	// project at runtime (same absolute path), so it needs no COPY or WORKDIR; it
	// only provides the base image and global tools, app deps come from the mount.
	container string
}

// knownRuntimes is the single source of truth for non-PHP runtime support.
// First match wins on detection. Node's manifests mirror isNodeProject so the
// two agree on what a Node project is.
var knownRuntimes = []projectRuntime{
	{
		label:      "Node",
		manifests:  []string{"package.json", ".nvmrc", ".node-version"},
		devCommand: "npm run dev",
		container:  "FROM node:20-alpine\nRUN npm install -g nodemon\nCMD [\"npm\", \"run\", \"dev\"]\n",
	},
	{
		label:      "Go",
		manifests:  []string{"go.mod"},
		devCommand: "go run .",
		container:  "FROM golang:1.23-alpine\n# Optional hot reload: RUN go install github.com/air-verse/air@latest (then CMD [\"air\"])\nCMD [\"go\", \"run\", \".\"]\n",
	},
	{
		label:      "Python",
		manifests:  []string{"pyproject.toml", "requirements.txt", "Pipfile", "manage.py"},
		devCommand: "python app.py",
		container:  "FROM python:3.12-slim\n# Install app deps at build time only if they aren't in the mounted project.\nCMD [\"python\", \"app.py\"]\n",
	},
	{
		label:      "Ruby",
		manifests:  []string{"Gemfile"},
		devCommand: "ruby app.rb",
		container:  "FROM ruby:3.3-alpine\nCMD [\"ruby\", \"app.rb\"]\n",
	},
	{
		label:      "Rust",
		manifests:  []string{"Cargo.toml"},
		devCommand: "cargo run",
		container:  "FROM rust:1-alpine\nCMD [\"cargo\", \"run\"]\n",
	},
}

// detectProjectRuntime returns the runtime whose manifest is present in cwd, so
// the wizard can give every language the same language-aware prompt instead of
// treating anything without a package.json as an unknown blob.
func detectProjectRuntime(cwd string) (*projectRuntime, bool) {
	for i := range knownRuntimes {
		for _, f := range knownRuntimes[i].manifests {
			if fileExists(filepath.Join(cwd, f)) {
				return &knownRuntimes[i], true
			}
		}
	}
	return nil, false
}

// defaultDevCommand guesses the host-proxy dev command from the detected
// runtime, refining Python to Django's runserver when manage.py is present
// (the table default of python app.py is wrong for Django/Flask). Returns "" for
// an unknown runtime so the wizard offers a blank (proxy-only) default.
func defaultDevCommand(cwd string) string {
	rt, ok := detectProjectRuntime(cwd)
	if !ok {
		return ""
	}
	if rt.label == "Python" && fileExists(filepath.Join(cwd, "manage.py")) {
		return "python manage.py runserver"
	}
	return rt.devCommand
}

// starterContainerfile returns a commented starter Containerfile.lerd tailored
// to the project's detected runtime. It's a scaffold for the user to edit; an
// unrecognised runtime gets a generic skeleton. See projectRuntime.container for
// why there is no COPY/WORKDIR.
func starterContainerfile(cwd string, port int) string {
	header := fmt.Sprintf("# Containerfile.lerd — lerd builds this image and runs your app in it.\n"+
		"# Your project is bind-mounted into the container at runtime (no COPY or WORKDIR\n"+
		"# needed) so your edits are live. Install global dev tools here; app dependencies\n"+
		"# come from the mounted project. Your app must listen on port %d.\n\n", port)
	body := "FROM alpine:latest\n# RUN <install the global dev tools your app needs>\n# CMD [\"<command that starts your server>\"]\n"
	if rt, ok := detectProjectRuntime(cwd); ok {
		body = rt.container
	}
	return header + body
}

// maybeCreateContainerfile offers to scaffold a missing Containerfile and open
// it in the user's editor. No-op when the file already exists or the shell is
// non-interactive (scripts/CI shouldn't block on an editor). Best-effort: a
// failure to write or open is warned, not fatal, since the link still proceeds.
func maybeCreateContainerfile(cwd, containerfile string, port int) {
	if !isInteractive() {
		return
	}
	path := filepath.Join(cwd, containerfile)
	if fileExists(path) {
		return
	}
	create := true
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("%s doesn't exist yet. Create it and open your editor?", containerfile)).
			Description("Lerd writes a starter image for your runtime; edit it to fit your app.").
			Value(&create),
	)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil || !create {
		return
	}
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	if err := os.WriteFile(path, []byte(starterContainerfile(cwd, port)), 0644); err != nil {
		feedback.Warn("could not create %s: %v", containerfile, err)
		return
	}
	fmt.Printf("Created %s\n", containerfile)
	if launched, err := launchEditor(path); err != nil {
		feedback.Warn("%v", err)
	} else if !launched {
		fmt.Printf("Set $EDITOR to edit it automatically; the starter file is at %s\n", containerfile)
	}
}

// resolveSecuredDefault computes a wizard's initial "secured" value and whether
// the HTTPS prompt should be offered at all. HTTPS is only available when lerd
// manages DNS; otherwise secured is forced off and the prompt is hidden so the
// wizard never offers a choice that `lerd secure` would later refuse.
//
// A project with nothing committed is a new one, and a new site answers the
// question with yes: the local CA is already trusted, so http is a choice worth
// making deliberately rather than the one an unread prompt makes. A project that
// is already configured keeps what it has, from .lerd.yaml or, when that never
// recorded the field, from the site as it is registered today.
func resolveSecuredDefault(cwd string, defaults *config.ProjectConfig, gcfg *config.GlobalConfig) (secured, httpsAvailable bool) {
	if !gcfg.DNSManaged() {
		return false, false
	}
	if defaults == nil || defaults.IsEmpty() {
		return true, true
	}
	if defaults.Secured {
		return true, true
	}
	if site, err := config.FindSiteByPath(cwd); err == nil && site.Secured {
		return true, true
	}
	return false, true
}

// persistedSecured keeps the user's HTTPS intent in .lerd.yaml even when DNS is
// disabled. Without DNS the wizard force-gates `secured` off, but the link path
// re-gates at runtime via ResolveSecured, so persisting the gated-off value
// would silently strip a committed `secured: true` for teammates on a
// DNS-managed box. When HTTPS is available we persist the user's choice.
func persistedSecured(chosen, httpsAvailable, committed bool) bool {
	if httpsAvailable {
		return chosen
	}
	return committed
}

// appendHTTPSField adds the "Enable HTTPS?" question to a wizard's field list
// only when HTTPS is available; in localhost mode the prompt is omitted. Shared
// by all three wizards so the gating rule lives in one place.
//
// It is a vertical picker rather than a yes/no confirm. The confirm renders both
// answers side by side as buttons and says which one is selected with background
// colour alone, which reads as a pair of labels; the picker marks the answer with
// a cursor, the way every other question in the wizard does.
func appendHTTPSField(fields []huh.Field, httpsAvailable bool, secured *bool) []huh.Field {
	if !httpsAvailable {
		return fields
	}
	return append(fields, huh.NewSelect[bool]().
		Title("Enable HTTPS?").
		Options(huh.NewOption("Yes", true), huh.NewOption("No", false)).
		Value(secured))
}

// validatePHPVersion checks that the input looks like a valid PHP version
// (e.g. "8.3", "8.4") and rejects inputs like "8,5" or plain strings.
func validatePHPVersion(s string) error {
	if !config.ValidPHPVersion(s) {
		return fmt.Errorf("PHP version must be in MAJOR.MINOR format, e.g. 8.3")
	}
	return nil
}

// detectServicesFromRules uses the FrameworkServiceDef detection rules from a
// framework YAML to determine which services are active.
func detectServicesFromRules(envFilePath, envFormat string, rules map[string]config.FrameworkServiceDef) []string {
	readKey := makeEnvReader(envFilePath, envFormat)

	var detected []string
	for _, svc := range knownServices() {
		def, ok := rules[svc]
		if !ok || len(def.Detect) == 0 {
			continue
		}
		for _, cond := range def.Detect {
			val := readKey(cond.Key)
			if val == "" {
				continue
			}
			if cond.ValuePrefix == "" || strings.HasPrefix(val, cond.ValuePrefix) {
				detected = append(detected, svc)
				break
			}
		}
	}
	return detected
}

// detectServicesHeuristic detects services for Laravel-style .env files where
// no explicit framework service detection rules are defined.
func detectServicesHeuristic(envFilePath, envFormat string) []string {
	readKey := makeEnvReader(envFilePath, envFormat)

	var detected []string

	dbConn := readKey("DB_CONNECTION")
	switch dbConn {
	case "mysql":
		detected = append(detected, "mysql")
	case "pgsql", "postgres":
		detected = append(detected, "postgres")
	}

	if v := readKey("REDIS_HOST"); v != "" && v != "null" && v != "127.0.0.1" && v != "localhost" {
		detected = append(detected, "redis")
	}

	if readKey("SCOUT_DRIVER") == "meilisearch" || readKey("MEILISEARCH_HOST") != "" {
		detected = append(detected, "meilisearch")
	}

	if readKey("FILESYSTEM_DISK") == "s3" && readKey("AWS_ENDPOINT") != "" {
		detected = append(detected, "rustfs")
	}

	if mailHost := readKey("MAIL_HOST"); mailHost == "lerd-mailpit" || readKey("MAIL_PORT") == "1025" {
		detected = append(detected, "mailpit")
	}

	return detected
}

// makeEnvReader returns a function that reads a single key from the env file,
// handling the dotenv, php-const, and php-array formats.
func makeEnvReader(envFilePath, envFormat string) func(key string) string {
	return envfile.Reader(envFilePath, envFormat)
}

// runSetupInit is called by lerd setup as its first step. It runs the init
// wizard when .lerd.yaml does not exist and we are in interactive mode, or
// silently applies the saved config when .lerd.yaml is already present.
// In non-interactive (--all) mode with no .lerd.yaml it falls back to a plain
// lerd link so setup can still run unattended.
func runSetupInit(cwd string, skipWizard bool) error {
	lerdYAMLPath := filepath.Join(cwd, ".lerd.yaml")
	_, statErr := os.Stat(lerdYAMLPath)
	hasExisting := statErr == nil

	if !hasExisting && skipWizard {
		// CI path: link with auto-detection, then run env so the caller
		// (lerd setup) doesn't have to do it itself. No interactive data import.
		linkSkipSetupPrompt = true
		linkSkipDataImport = true
		defer func() { linkSkipSetupPrompt = false; linkSkipDataImport = false }()
		if err := runLink([]string{}); err != nil {
			return err
		}
		runEnvIfManaged(cwd, func() error { return runEnv(nil, nil) })
		return nil
	}

	if !hasExisting {
		existing, _ := config.LoadProjectConfig(cwd)
		existing = applyImportSeed(cwd, existing)
		cfg, err := runWizard(cwd, existing)
		if err != nil {
			return err
		}
		write := feedback.Start("writing .lerd.yaml")
		if err := config.SaveProjectConfig(cwd, cfg); err != nil {
			write.Fail(err)
			return fmt.Errorf("saving .lerd.yaml: %w", err)
		}
		write.OK("")
	}

	return applyProjectConfig(cwd)
}

// settledRegistration returns the site registered for cwd when a link would
// write exactly what the registry already holds, so there is nothing to apply.
// The plan is resolved without a prompter: a question asked here would be asked
// about a site that is already serving, and a framework detection that comes up
// short simply reads as a change and lets the link run as before.
func settledRegistration(cwd string) (config.Site, bool) {
	existing, err := config.FindSiteByPath(cwd)
	if err != nil || existing == nil {
		return config.Site{}, false
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return config.Site{}, false
	}
	plan, err := linker.Resolve(cwd, cfg, linker.CLIPolicy("", false, nil))
	if err != nil || !plan.MatchesRegistration(*existing) {
		return config.Site{}, false
	}
	return *existing, true
}

// siteAddress is the URL a site answers on, for the line that reports a link
// there was no need to run.
func siteAddress(site config.Site) string {
	scheme := "http"
	if site.Secured {
		scheme = "https"
	}
	return scheme + "://" + site.PrimaryDomain()
}

func applyProjectConfig(cwd string) error {
	// Suppress the "Run lerd setup?" prompt and the link summary inside runLink —
	// we're already in init/setup, the caller handles worker steps, and the
	// summary is printed here after the .env step so it lands last.
	linkSkipSetupPrompt = true
	linkSkipSummary = true
	defer func() { linkSkipSetupPrompt = false; linkSkipSummary = false }()

	start := time.Now()
	proj, err := config.LoadProjectConfig(cwd)
	if err != nil {
		return err
	}

	// Skip work that already ran earlier in this process. When a `lerd link`
	// flows into `lerd setup` via the prompt, the link (and often .env) is
	// already done, so re-running it would just repeat the same output.
	ranLink := false
	if !linkApplied {
		// linkApplied above covers one command doing both halves; this covers two.
		// A link that would write the registration the registry already holds has
		// no work in it, and running it anyway repeats every provisioning step and
		// the summary for a site that is already being served.
		if site, settled := settledRegistration(cwd); settled {
			feedback.Start("already linked").OK(feedback.Val(siteAddress(site)))
			linkApplied = true
		} else {
			// Install PHP FPM with a progress loader if the version is not yet installed.
			// runLink handles everything else (framework restore, node-version, secure, services).
			if proj.PHPVersion != "" && !phpPkg.IsInstalled(proj.PHPVersion) {
				phpVersion := proj.PHPVersion
				jobs := []BuildJob{{
					Label: "PHP " + phpVersion + " FPM",
					Run: func(w io.Writer) error {
						return ensureFPMQuadletTo(phpVersion, w)
					},
				}}
				if err := RunParallel(jobs); err != nil {
					feedback.Warn("PHP %s FPM: %v", phpVersion, err)
				}
			}

			if err := runLink([]string{}); err != nil {
				return err
			}
			ranLink = true
		}
	}

	// Apply the wizard's service choices (database, etc.) to .env so the user
	// sees DB_CONNECTION/DB_HOST/etc. updated immediately after the wizard.
	if !envApplied {
		applyEnvStep(cwd)
	}

	// Print the deferred link summary now, so it reads as the final word after
	// every provisioning step (including .env) has reported. Skip it when we
	// didn't link this pass — the summary was already shown.
	linkSkipSummary = false
	if ranLink {
		if site, err := config.FindSiteByPath(cwd); err == nil {
			printLinkSummary(*site, start, syncIDEDataSource(site.Path).wrote())
		}
	}
	return nil
}

// applyEnvStep runs `lerd env` quietly under a single condensed feedback step,
// listing the services it configured (from envSummary) rather than its full
// per-service output.
func applyEnvStep(cwd string) {
	envApplied = true
	if !feedback.Interactive() {
		runEnvIfManaged(cwd, func() error { return runEnv(nil, nil) })
		return
	}
	runEnvIfManaged(cwd, func() error { return runEnvLive(nil, nil) })
}

// runCapturingStdout redirects os.Stdout to a pipe for the duration of fn and
// returns everything it wrote. A reader goroutine drains continuously so even
// large output (composer/npm) never blocks on the pipe buffer. Used to fold a
// setup step's verbose output behind a single feedback line, surfacing it only
// when the step fails.
func runCapturingStdout(fn func() error) ([]byte, error) {
	prevOut, prevErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fn()
	}
	// Swap both streams: subprocesses (npm/vite, artisan) write progress and
	// warnings to stderr too (e.g. Browserslist), and a leaked stderr line would
	// clobber the live spinner the caller animates on the real stdout.
	os.Stdout = w
	os.Stderr = w
	captured := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(r)
		captured <- b
	}()
	runErr := fn()
	os.Stdout = prevOut
	os.Stderr = prevErr
	_ = w.Close()
	return <-captured, runErr
}
