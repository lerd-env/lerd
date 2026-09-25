package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
	nodeDet "github.com/geodro/lerd/internal/node"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/store"
)

// The kinds of project the init questions come in. A directory is one of them
// before a single question is asked: how lerd serves it decides what there is
// to ask about.
const (
	ProjectKindPHP       = "php"
	ProjectKindProxy     = "proxy"
	ProjectKindContainer = "container"
)

// ChoiceOption is one selectable answer: the value that gets saved and the
// label to show for it.
type ChoiceOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ProjectQuestions is what `lerd init` asks about a directory, as data rather
// than as a terminal form: the options to offer and the answers to start from.
// The terminal wizard renders it with huh and the dashboard renders it as a
// step in the site wizard, so both ask the same questions of the same project.
type ProjectQuestions struct {
	Dir  string `json:"dir"`
	Kind string `json:"kind"`

	// KindChoice marks a directory lerd cannot classify on its own, where the
	// first question is how to run it at all. KindOptions is what to offer.
	KindChoice  bool           `json:"kind_choice"`
	KindOptions []ChoiceOption `json:"kind_options,omitempty"`
	KindTitle   string         `json:"kind_title,omitempty"`

	Framework      string `json:"framework,omitempty"`
	FrameworkLabel string `json:"framework_label,omitempty"`

	PHPVersion    string   `json:"php_version,omitempty"`
	PHPInstalled  []string `json:"php_installed,omitempty"`
	NodeManaged   bool     `json:"node_managed"`
	NodeVersion   string   `json:"node_version,omitempty"`
	NodeInstalled []string `json:"node_installed,omitempty"`
	NodeVersionOf string   `json:"node_version_of,omitempty"`

	HTTPSAvailable bool `json:"https_available"`
	Secured        bool `json:"secured"`

	DatabaseOptions []ChoiceOption `json:"database_options,omitempty"`
	Database        string         `json:"database,omitempty"`
	ServiceOptions  []string       `json:"service_options,omitempty"`
	Services        []string       `json:"services,omitempty"`

	FrankenPHPOffered bool   `json:"frankenphp_offered"`
	FrankenPHPReason  string `json:"frankenphp_reason,omitempty"`
	FrankenPHP        bool   `json:"frankenphp"`
	FrankenPHPWorker  bool   `json:"frankenphp_worker"`

	WorkerOptions []string `json:"worker_options,omitempty"`
	Workers       []string `json:"workers,omitempty"`

	ProxyCommand     string `json:"proxy_command,omitempty"`
	ProxyCommandHint string `json:"proxy_command_hint,omitempty"`
	ProxyPort        int    `json:"proxy_port,omitempty"`
	ProxyVitePitfall bool   `json:"proxy_vite_pitfall,omitempty"`

	ContainerPort int    `json:"container_port,omitempty"`
	Containerfile string `json:"containerfile,omitempty"`
}

// ProjectAnswers is a filled-in ProjectQuestions: what the user chose, in the
// shape .lerd.yaml is written from.
type ProjectAnswers struct {
	Kind string `json:"kind"`

	PHPVersion  string `json:"php_version"`
	NodeVersion string `json:"node_version"`
	Secured     bool   `json:"secured"`

	Database string   `json:"database"`
	Services []string `json:"services"`

	FrankenPHP       bool `json:"frankenphp"`
	FrankenPHPWorker bool `json:"frankenphp_worker"`

	Workers []string `json:"workers"`

	ProxyCommand string `json:"proxy_command"`
	ProxyPort    int    `json:"proxy_port"`

	ContainerPort int    `json:"container_port"`
	Containerfile string `json:"containerfile"`
}

// projectKindFor classifies a directory the way runWizard routes it: a saved
// section wins, then a Containerfile with no PHP, then whether there is a PHP
// project here at all. A directory nothing recognises is left to the user, and
// says so through KindChoice.
func projectKindFor(cwd string, defaults *config.ProjectConfig) (kind string, choice bool, options []ChoiceOption, title string) {
	proxyOption := ChoiceOption{Value: ProjectKindProxy, Label: "Dev server (proxy to a host port)"}
	containerOption := ChoiceOption{Value: ProjectKindContainer, Label: "Custom container (Containerfile.lerd)"}
	phpOption := ChoiceOption{Value: ProjectKindPHP, Label: "Plain PHP site"}

	if defaults.Proxy != nil {
		return ProjectKindProxy, false, nil, ""
	}
	if defaults.Container != nil {
		return ProjectKindContainer, false, nil, ""
	}

	_, hasFramework := resolveFramework(cwd)
	hasComposer := fileExists(filepath.Join(cwd, "composer.json"))
	if hasFramework || hasComposer {
		return ProjectKindPHP, false, nil, ""
	}
	if podman.HasContainerfile(cwd) {
		return ProjectKindContainer, false, nil, ""
	}

	if rt, known := detectProjectRuntime(cwd); known {
		return ProjectKindProxy, true,
			[]ChoiceOption{proxyOption, containerOption},
			fmt.Sprintf("This looks like a %s project. How should lerd run it?", rt.label)
	}
	return ProjectKindProxy, true,
		[]ChoiceOption{proxyOption, containerOption, phpOption},
		"No PHP project detected. How should lerd run it?"
}

// ProjectQuestionsFor builds the questions for a directory, seeded from its
// saved .lerd.yaml (and from whatever an imported Sail or Valet project already
// declares), so re-running them shows what the project committed to rather than
// a blank form.
func ProjectQuestionsFor(cwd string) (*ProjectQuestions, error) {
	gcfg, err := config.LoadGlobal()
	if err != nil {
		return nil, err
	}
	defaults, err := config.LoadProjectConfig(cwd)
	if err != nil {
		return nil, err
	}
	defaults = applyImportSeed(cwd, defaults)

	kind, choice, kindOptions, kindTitle := projectKindFor(cwd, defaults)
	secured, httpsAvailable := resolveSecuredDefault(cwd, defaults, gcfg)

	q := &ProjectQuestions{
		Dir:            cwd,
		Kind:           kind,
		KindChoice:     choice,
		KindOptions:    kindOptions,
		KindTitle:      kindTitle,
		HTTPSAvailable: httpsAvailable,
		Secured:        secured,
		Services:       defaults.ServiceNames(),
	}

	// A directory the user is being asked to classify gets every kind's answers
	// filled in, so switching the first question over shows defaults rather than
	// empty fields the save would then refuse.
	if choice || kind == ProjectKindPHP {
		fillPHPQuestions(q, cwd, defaults, gcfg)
	}
	if choice || kind == ProjectKindProxy {
		fillProxyQuestions(q, cwd, defaults)
	}
	if choice || kind == ProjectKindContainer {
		fillContainerQuestions(q, defaults)
	}
	fillServiceQuestions(q, cwd, defaults)
	return q, nil
}

// fillServiceQuestions splits what the project uses into the database answer
// and the rest. Every kind is asked: a dev server or a container has no
// database question of its own on the terminal, but it can still declare the
// service, and answering it in one place keeps the saved file the same.
func fillServiceQuestions(q *ProjectQuestions, cwd string, defaults *config.ProjectConfig) {
	fw, _ := config.GetFrameworkForDir(q.Framework, cwd)
	dbOptions, dbNameSet := buildDatabaseOptions(fw)
	for _, opt := range dbOptions {
		q.DatabaseOptions = append(q.DatabaseOptions, ChoiceOption{Value: opt.Value, Label: opt.Key})
	}
	q.ServiceOptions = nonDatabaseServiceNames(dbNameSet)
	q.Database, q.Services = wizardServiceDefaults(cwd, defaults, dbNameSet)
	q.ServiceOptions, q.Services = addSuggestedServices(q.ServiceOptions, q.Services, fw, dbNameSet, len(defaults.Services) > 0, presetAvailable, serviceInstalled)
}

// addSuggestedServices offers the presets the framework and its packages
// suggest. Each package puts one of its alternatives forward, the first
// installed here or else its first, ticked on a project that has not saved its
// services yet, since requiring the package is evidence the project uses it.
func addSuggestedServices(options, selected []string, fw *config.Framework, dbNameSet map[string]bool, saved bool, available, installed func(string) bool) ([]string, []string) {
	if fw == nil {
		return options, selected
	}
	picked := config.PickPackageSuggestions(fw.PackageServices, func(name string) bool { return slices.Contains(selected, name) }, installed)
	for _, sg := range append(append([]config.ServiceSuggestion(nil), fw.SuggestServices...), picked...) {
		name := sg.Name
		if name == "" || dbNameSet[name] || !available(name) {
			continue
		}
		if !slices.Contains(options, name) {
			options = append(options, name)
		}
	}
	for _, sg := range picked {
		if !saved && slices.Contains(options, sg.Name) && !slices.Contains(selected, sg.Name) {
			selected = append(selected, sg.Name)
		}
	}
	return options, selected
}

// serviceInstalled reports whether this machine already runs the service.
func serviceInstalled(name string) bool {
	return podman.QuadletInstalled("lerd-" + name)
}

// presetAvailable reports whether name is a preset lerd can install, fetching
// it from the store when this machine has not cached it, so that picking it
// records a preset that link installs rather than a name that resolves to nothing.
func presetAvailable(name string) bool {
	if config.PresetExists(name) {
		return true
	}
	_, err := store.NewServiceClient().FetchServicePreset(name)
	return err == nil
}

// fillPHPQuestions fills in what a PHP project is asked: the version to serve
// it with, HTTPS, its database and services, FrankenPHP where the project hints
// at it, and the workers its framework definition declares.
func fillPHPQuestions(q *ProjectQuestions, cwd string, defaults *config.ProjectConfig, gcfg *config.GlobalConfig) {
	framework, _ := resolveFramework(cwd)
	q.Framework = framework
	if fw, ok := config.GetFrameworkForDir(framework, cwd); ok {
		q.FrameworkLabel = fw.Label
	}

	q.PHPVersion = wizardPHPDefault(cwd, defaults, gcfg, framework)
	q.PHPInstalled, _ = phpPkg.ListInstalled()

	if lerdManagesNode() {
		resolved, source := nodeDet.UnpinnedVersion(cwd)
		q.NodeManaged = true
		q.NodeVersion = nodeVersionDefault(defaults.NodeVersion, resolved)
		q.NodeInstalled = nodeDet.ListInstalled()
		q.NodeVersionOf = source
	}

	if hints := config.DetectFrankenPHPHints(cwd); len(hints) > 0 || defaults.Runtime == "frankenphp" {
		q.FrankenPHPOffered = true
		q.FrankenPHPReason = "Detected FrankenPHP signals in this project"
		if len(hints) > 0 {
			q.FrankenPHPReason = hints[0].Reason
		}
		q.FrankenPHP = defaults.Runtime == "frankenphp"
		q.FrankenPHPWorker = defaults.RuntimeWorker
	}

	q.WorkerOptions = frameworkWorkerOptions(cwd, framework, nil)
	q.Workers = keepAvailable(defaults.Workers, q.WorkerOptions)
}

// fillProxyQuestions fills in what a dev-server project is asked: the command
// lerd supervises and the port it binds.
func fillProxyQuestions(q *ProjectQuestions, cwd string, defaults *config.ProjectConfig) {
	manifest := readPackageManifest(cwd)
	devScripts := manifest.devScripts()

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
	q.ProxyCommand = command
	if len(devScripts) > 0 {
		q.ProxyCommandHint = strings.Join(devScripts, ", ")
	}

	port := 0
	if defaults.Proxy != nil {
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
	q.ProxyPort = port
	q.ProxyVitePitfall = manifest.runsVite(command)
}

// fillContainerQuestions fills in what a custom-container project is asked: the
// port the app listens on inside the container and the Containerfile to build.
func fillContainerQuestions(q *ProjectQuestions, defaults *config.ProjectConfig) {
	q.ContainerPort = 3000
	q.Containerfile = "Containerfile.lerd"
	if defaults.Container != nil {
		if defaults.Container.Port > 0 {
			q.ContainerPort = defaults.Container.Port
		}
		if defaults.Container.Containerfile != "" {
			q.Containerfile = defaults.Container.Containerfile
		}
	}
}

// wizardPHPDefault resolves the PHP version the questions start on: a saved
// answer, else the site registry, else what the project itself resolves to,
// clamped into the range its framework declares.
func wizardPHPDefault(cwd string, defaults *config.ProjectConfig, gcfg *config.GlobalConfig, framework string) string {
	version := defaults.PHPVersion
	if version == "" {
		if site, err := config.FindSiteByPath(cwd); err == nil {
			version = site.PHPVersion
		}
	}
	if version == "" {
		if v, err := phpPkg.DetectVersion(cwd); err == nil {
			version = v
		} else {
			version = gcfg.PHP.DefaultVersion
		}
	}
	phpMin, phpMax := "", ""
	if framework != "" {
		// Skip a guessed definition's range so a legacy project keeps its real
		// detected default (Laravel 6 on 7.4, not the borrowed Laravel 10 8.1).
		if fw, ok := config.GetFrameworkForDir(framework, cwd); ok && !fw.VersionGuessed {
			phpMin, phpMax = fw.PHP.Min, fw.PHP.Max
		}
	}
	return phpPkg.ClampToRange(version, phpMin, phpMax)
}

// nonDatabaseServiceNames lists the services offered outside the database
// question: the default presets plus custom services the project's code can
// actually consume, minus anything that belongs to a database family.
func nonDatabaseServiceNames(dbNameSet map[string]bool) []string {
	options := make([]string, 0)
	for _, svc := range knownServices() {
		if !dbNameSet[svc] {
			options = append(options, svc)
		}
	}
	if customs, err := config.ListCustomServices(); err == nil {
		for _, svc := range customs {
			if dbNameSet[svc.Name] {
				continue
			}
			// Skip developer tools the project's code never consumes
			// (phpMyAdmin, pgAdmin, mongo-express): no env_vars and no
			// env_detect means nothing to wire into .env.
			if len(svc.EnvVars) == 0 && svc.EnvDetect == nil {
				continue
			}
			options = append(options, svc.Name)
		}
	}
	return options
}

// wizardServiceDefaults splits what the project already uses into the database
// answer and the rest, falling back to what its env file says when nothing is
// saved or detected.
func wizardServiceDefaults(cwd string, defaults *config.ProjectConfig, dbNameSet map[string]bool) (string, []string) {
	serviceDefaults := defaults.ServiceNames()
	if len(serviceDefaults) == 0 {
		serviceDefaults = detectServicesFromDir(cwd)
	}

	dbChoice := "sqlite"
	for _, name := range serviceDefaults {
		if dbNameSet[name] {
			dbChoice = name
			break
		}
	}
	if dbChoice == "sqlite" {
		switch detectDBConnection(cwd) {
		case "mysql", "mariadb":
			dbChoice = "mysql"
		case "pgsql", "postgres":
			dbChoice = "postgres"
		}
	}

	nonDB := make([]string, 0, len(serviceDefaults))
	for _, name := range serviceDefaults {
		if !dbNameSet[name] {
			nonDB = append(nonDB, name)
		}
	}
	return dbChoice, nonDB
}

// frameworkWorkerOptions lists the workers a directory can auto-start: the ones
// its framework definition declares whose check passes, minus those a declared
// worker conflicts with (horizon replaces queue), plus stripe when the project
// carries a secret for it. removedCustom names custom workers the caller is
// dropping, whose conflict rules no longer apply.
func frameworkWorkerOptions(cwd, framework string, removedCustom map[string]bool) []string {
	var options []string
	if fw, ok := config.GetFrameworkForDir(framework, cwd); ok && fw.Workers != nil {
		suppressed := map[string]bool{}
		for name, wDef := range fw.Workers {
			if removedCustom[name] {
				continue
			}
			if wDef.Check != nil && !config.MatchesRule(cwd, *wDef.Check) {
				continue
			}
			for _, c := range wDef.ConflictsWith {
				suppressed[c] = true
			}
		}
		for name, wDef := range fw.Workers {
			if removedCustom[name] || suppressed[name] {
				continue
			}
			if wDef.Check != nil && !config.MatchesRule(cwd, *wDef.Check) {
				continue
			}
			options = append(options, name)
		}
		sort.Strings(options)
	}
	if StripeSecretSet(cwd) {
		options = append(options, "stripe")
	}
	return options
}

// keepAvailable drops selections the options no longer offer, so a saved worker
// whose framework definition dropped it doesn't come back pre-ticked.
func keepAvailable(selected, options []string) []string {
	available := make(map[string]bool, len(options))
	for _, o := range options {
		available[o] = true
	}
	kept := make([]string, 0, len(selected))
	for _, s := range selected {
		if available[s] {
			kept = append(kept, s)
		}
	}
	return kept
}

// SaveProjectAnswers writes the answers to .lerd.yaml, the same file the
// terminal wizard writes, so a project configured from the dashboard is
// portable and applies identically on the next link.
func SaveProjectAnswers(cwd string, answers ProjectAnswers) error {
	gcfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	defaults, err := config.LoadProjectConfig(cwd)
	if err != nil {
		return err
	}
	defaults = applyImportSeed(cwd, defaults)
	_, httpsAvailable := resolveSecuredDefault(cwd, defaults, gcfg)

	cfg, err := projectConfigFromAnswers(cwd, defaults, answers, httpsAvailable)
	if err != nil {
		return err
	}
	return config.SaveProjectConfig(cwd, cfg)
}

// projectConfigFromAnswers turns answers into the .lerd.yaml a link applies,
// carrying over the parts of an existing config the questions never ask about
// (the public dir, the app URL, extra domains, custom workers).
func projectConfigFromAnswers(cwd string, defaults *config.ProjectConfig, a ProjectAnswers, httpsAvailable bool) (*config.ProjectConfig, error) {
	// An empty answer means two different things. The dashboard only renders
	// the Node question for a PHP project on a machine where lerd manages Node,
	// and there it offers a "Not pinned" entry whose value is exactly this empty
	// string, so empty is the user clearing the pin. Everywhere else, proxy and
	// container kinds included, the question was never asked and empty must not
	// erase what the project pins.
	nodeVersion := a.NodeVersion
	if nodeVersion == "" && !(a.Kind == ProjectKindPHP && lerdManagesNode()) {
		nodeVersion = defaults.NodeVersion
	}

	cfg := &config.ProjectConfig{
		PublicDir:     defaults.PublicDir,
		Secured:       persistedSecured(a.Secured, httpsAvailable, defaults.Secured),
		AppURL:        defaults.AppURL,
		Domains:       defaults.Domains,
		NodeVersion:   nodeVersion,
		CustomWorkers: defaults.CustomWorkers,
	}

	switch a.Kind {
	case ProjectKindProxy:
		if a.ProxyPort <= 0 {
			return nil, fmt.Errorf("a dev server needs the port it listens on")
		}
		cfg.Services = buildProjectServices(persistedServices(a.Database, a.Services), defaults)
		cfg.Proxy = &config.ProxyConfig{Command: a.ProxyCommand, Port: a.ProxyPort}
		if defaults.Proxy != nil {
			cfg.Proxy.SSL = defaults.Proxy.SSL
			cfg.Proxy.PortEnvKey = defaults.Proxy.PortEnvKey
			cfg.Proxy.HostEnvKey = defaults.Proxy.HostEnvKey
			cfg.Proxy.InjectHost = defaults.Proxy.InjectHost
		}
		return cfg, nil

	case ProjectKindContainer:
		if a.ContainerPort <= 0 {
			return nil, fmt.Errorf("a custom container needs the port the app listens on")
		}
		containerfile := a.Containerfile
		if containerfile == "" {
			containerfile = "Containerfile.lerd"
		}
		cfg.Services = buildProjectServices(persistedServices(a.Database, a.Services), defaults)
		cfg.Container = &config.ContainerConfig{Port: a.ContainerPort, Containerfile: containerfile}
		if defaults.Container != nil {
			cfg.Container.BuildContext = defaults.Container.BuildContext
			cfg.Container.Target = defaults.Container.Target
			cfg.Container.SSL = defaults.Container.SSL
		}
		return cfg, nil
	}

	framework, _ := resolveFramework(cwd)
	cfg.PHPVersion = a.PHPVersion
	cfg.Framework = framework
	cfg.Services = buildProjectServices(persistedServices(a.Database, a.Services), defaults)
	cfg.Workers = a.Workers

	// Only embed the framework definition for user-defined frameworks that
	// aren't available from the store; built-in and store-installed ones can be
	// fetched on any machine.
	if framework != "" {
		if config.GetFrameworkSource(framework) == config.SourceUser {
			if fw, ok := config.GetFramework(framework); ok {
				cfg.FrameworkDef = fw
			}
		}
	}
	if cfg.FrameworkDef != nil && cfg.FrameworkDef.Version != "" {
		cfg.FrameworkVersion = cfg.FrameworkDef.Version
	} else if fw, ok := config.GetFrameworkForDir(framework, cwd); ok && fw.Version != "" {
		cfg.FrameworkVersion = fw.Version
	}

	if a.FrankenPHP {
		cfg.Runtime = "frankenphp"
		cfg.RuntimeWorker = a.FrankenPHPWorker
	}
	return cfg, nil
}
