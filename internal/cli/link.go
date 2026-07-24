package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/linker"
	"github.com/geodro/lerd/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// linkSkipSetupPrompt suppresses the post-link "Run lerd setup?" suggestion
// when runLink is called from within lerd setup / lerd init (prevents infinite
// recursion and a redundant nag while setup is already running).
var linkSkipSetupPrompt bool

// linkSkipSummary defers runLink's success line and details block to the
// caller. Set by the init/setup flow so the ".env" step prints before the
// "linked" summary rather than after it.
var linkSkipSummary bool

// linkSkipDataImport suppresses the Sail data-import offer. It is kept separate
// from linkSkipSetupPrompt so routing `lerd link` through the init wizard
// (which suppresses the setup prompt) still offers to import an existing Sail
// database. Only the unattended (--all/CI) path sets it.
var linkSkipDataImport bool

// linkApplied and envApplied track whether the current process has already
// linked the site and applied its .env. When a link flows straight into setup
// (the "Run lerd setup?" prompt), this lets the setup pass skip re-printing the
// same provisioning steps and jump to the step selector.
var (
	linkApplied bool
	envApplied  bool
)

// linkAssumeYes approves a host-proxy dev command without the interactive
// confirmation prompt. Set by `lerd link --yes` and by the UI link flow, where
// the user's explicit action is the consent.
var linkAssumeYes bool

// presetVersionSuffix returns " (5.7)" for a non-empty version, otherwise "".
func presetVersionSuffix(version string) string {
	if version == "" {
		return ""
	}
	return " (" + version + ")"
}

// NewLinkCmd returns the link command.
func NewLinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link [domain]",
		Short: "Link the current directory as a site",
		Long:  "Register the current directory as a lerd site. The optional argument is the domain name without the TLD (e.g. 'myapp' becomes myapp.test). Defaults to the directory name.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLinkOrInit(args)
		},
	}
	cmd.Flags().BoolVar(&linkAssumeYes, "yes", false, "Approve a host-proxy dev command without the confirmation prompt")
	return cmd
}

// runLinkOrInit routes a user-typed `lerd link` into the init wizard when the
// project has no usable .lerd.yaml and we have an interactive terminal, so a
// fresh link guides the user through configuration (the wizard then links and
// offers setup) instead of leaving a bare, unconfigured registration. Every
// other case — a configured .lerd.yaml, a non-interactive shell (park, CI,
// scripts), or an explicit domain argument — falls through to a direct link.
// Internal runLink callers bypass this routing entirely.
func runLinkOrInit(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	// IsEmpty (not file existence) so a present-but-empty .lerd.yaml still routes
	// to the wizard, matching what runLink's old empty-project branch did.
	proj, _ := config.LoadProjectConfig(cwd)
	hasConfig := proj != nil && !proj.IsEmpty()
	_, _, isWorktree := findOwningWorktree(cwd)
	if linkShouldRunWizard(hasConfig, isInteractive(), len(args) > 0, isWorktree) {
		// fresh=true: linkShouldRunWizard already gated on !hasConfig (absent or
		// empty .lerd.yaml), so force the wizard. Without it runInit re-decides on
		// file existence and a present-but-empty file would skip the wizard into a
		// bare link, contradicting the IsEmpty routing above.
		return runInit(true)
	}
	return runLink(args)
}

// linkShouldRunWizard reports whether a user-invoked `lerd link` should run the
// init wizard rather than a bare link. Only true for a fresh, interactive,
// argument-free link on a real site directory: a missing .lerd.yaml means
// nothing is committed yet, the terminal can host the wizard, no explicit
// domain was requested, and the directory isn't a worktree (which inherits its
// parent's registration). Any false input keeps the fast, scriptable link.
func linkShouldRunWizard(hasConfig, interactive, hasDomainArg, isWorktree bool) bool {
	return !hasConfig && interactive && !hasDomainArg && !isWorktree
}

// linkShouldImportSail reports whether runLink should offer to import an
// existing Sail project's data. Gated on a live terminal and a dedicated
// suppression flag (NOT linkSkipSetupPrompt), so a fresh `lerd link` that
// routes through the init wizard still offers the import.
func linkShouldImportSail(interactive, skipImport, hasSail bool) bool {
	return interactive && !skipImport && hasSail
}

func runLink(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	start := time.Now()
	// A standalone link opens the feedback block with a blank line for breathing
	// room; when embedded in init/setup the caller already printed one.
	if !linkSkipSetupPrompt {
		feedback.Begin()
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	// Load .lerd.yaml early so its values can influence the link.
	proj, _ := config.LoadProjectConfig(cwd)

	// Restore embedded custom framework definition before the framework is
	// resolved. The embedded def in .lerd.yaml is the project's known-good
	// configuration. Compare against whichever definition is currently active
	// (user-defined or store-installed).
	if proj != nil && proj.Framework != "" && proj.FrameworkDef != nil {
		proj.FrameworkDef.Name = proj.Framework
		// The embedded def is untrusted; sanitise the copy we'd install so a
		// .lerd.yaml can't seed host-executing doctor checks, and so the conflict
		// prompt compares like-for-like with what is (or would be) in the store.
		safe := config.SanitizeProjectFrameworkDef(proj.FrameworkDef)
		existing, exists := config.GetFrameworkForDir(proj.Framework, cwd)
		if !exists {
			// No definition anywhere — save the embedded one to the store dir.
			_ = config.SaveStoreFramework(safe)
		} else {
			action, err := confirmReplace("framework", proj.Framework, existing, safe)
			if err != nil {
				return err
			}
			switch action {
			case replaceFromProject:
				// User chose the .lerd.yaml version — save to store dir.
				_ = config.SaveStoreFramework(safe)
			case replaceFromDisk:
				// User chose the local/store version — update .lerd.yaml.
				_ = config.SetProjectFrameworkDef(cwd, existing)
			}
		}
	}

	// Write .node-version from .lerd.yaml if the file is not already present.
	if proj != nil && proj.NodeVersion != "" {
		nodeVersionFile := filepath.Join(cwd, ".node-version")
		if _, statErr := os.Stat(nodeVersionFile); os.IsNotExist(statErr) {
			_ = os.WriteFile(nodeVersionFile, []byte(proj.NodeVersion+"\n"), 0644)
		}
	}

	requested := ""
	if len(args) > 0 {
		requested = args[0]
	}
	policy := linkPolicy(requested)
	plan, err := linker.Resolve(cwd, cfg, policy)
	if err != nil {
		return err
	}
	if plan.Skip == linker.SkipWorktree {
		fmt.Printf("This directory is the %q worktree of site %q.\n", plan.WorktreeBranch, plan.WorktreeParent.Name)
		fmt.Printf("Worktrees inherit the parent's registration; not linking %s as a separate site.\n", cwd)
		fmt.Printf("Manage it from the parent (%s) or via `lerd worktree`.\n", plan.WorktreeParent.Path)
		return nil
	}

	servesPHP := plan.Mode != linker.ModeCustomContainer && plan.Mode != linker.ModeHostProxy
	if servesPHP {
		fwStep := feedback.Start("detecting framework")
		if plan.Site.Framework != "" {
			fwStep.OK(plan.FrameworkLabel)
		} else {
			fwStep.Info(plan.FrameworkLabel)
		}

		// Fold in the framework's required services before the runtime is applied,
		// so a custom-FPM or FrankenPHP site gets them too, not only the standard
		// PHP-FPM path.
		if fw, ok := config.GetFrameworkForDir(plan.Site.Framework, cwd); ok {
			proj = ensureRequiredServices(cwd, proj, fw, presetResolvable)
			plan.Project = proj
		}
	}

	res, err := linker.Apply(plan, policy, linkDeps(), cliReporter{})
	if err != nil {
		return err
	}
	site := res.Site
	printLinkSummary(site, start, res.WroteIDEDataSource)

	if plan.Mode != linker.ModeFPM {
		return linkApplyServices(cwd, proj)
	}

	// Warn before the setup prompt below: an ext-* requirement the image cannot
	// satisfy fails composer install, which is the first thing setup runs.
	warnMissingExtensions(cwd, site.Name, site.PHPVersion, cfg)

	// Sail detection — offer to import data before setup so lerd's DB is
	// populated from the existing Sail environment. Gate on Sail actually being
	// initialized (a compose file), not just the laravel/sail dev dependency
	// that every fresh Laravel app ships, which would prompt on a new project.
	if linkShouldImportSail(isInteractive(), linkSkipDataImport, sailInitialized(cwd)) {
		sailDBName := sailLinkDetectDBName(cwd)
		if feedback.Confirm("This project uses Laravel Sail. Import database (and S3 files) from Sail into lerd?", false) {
			if err := runImportSail(false, false, "sail", "password", sailDBName, sailDBName != "", false, false); err != nil {
				feedback.Warn("sail import: %v", err)
			}
		}
	}

	if hint, suggest := linkNextStep(linkSkipSetupPrompt); suggest {
		if isInteractive() {
			if feedback.Confirm("Run lerd setup?", true) {
				if err := runSetup(false, false); err != nil {
					feedback.Warn("setup: %v", err)
				}
			}
		} else {
			fmt.Println(hint)
		}
	}

	if proj != nil && shouldSecureOnLink(proj.Secured, site.Secured, cfg.DNSManaged()) {
		if err := runSecure(nil, []string{}); err != nil {
			feedback.Warn("securing site: %v", err)
		}
	}

	return linkApplyServices(cwd, proj)
}

// printLinkSummary prints the green success line and an aligned details block,
// deriving every field from the registered site so callers don't repeat them.
func printLinkSummary(site config.Site, start time.Time, wroteDataSource bool) {
	// Reached on every successful link path, so this is where we record that the
	// site is linked for this process (even when the summary itself is deferred).
	linkApplied = true
	if linkSkipSummary {
		return
	}
	feedback.Success("linked", time.Since(start))

	scheme := "http"
	if site.Secured {
		scheme = "https"
	}
	sum := feedback.NewSummary().Row("Site", feedback.Val(scheme+"://"+site.PrimaryDomain()))
	switch {
	case site.HostPort > 0:
		sum.Row("Serving", fmt.Sprintf("host proxy · port %d", site.HostPort))
	case site.ContainerPort > 0:
		sum.Row("Serving", fmt.Sprintf("custom container · port %d", site.ContainerPort))
	case site.PHPVersion != "":
		sum.Row("PHP", feedback.Val(site.PHPVersion)+" · "+linkRuntimeLabel(site))
	}
	if site.NodeVersion != "" {
		sum.Row("Node", feedback.Val(site.NodeVersion))
	}
	if site.Framework != "" {
		sum.Row("Framework", site.Framework)
	}
	readEnv := summaryEnvReader(site)
	if db := strings.TrimSpace(readEnv("DB_CONNECTION")); db != "" {
		if cache := strings.TrimSpace(readEnv("CACHE_STORE")); cache != "" {
			db += " · cache " + cache
		}
		sum.Row("DB", db)
	}
	if wroteDataSource {
		sum.Row("IDE", "database connection written to .idea")
	}
	sum.Print()
}

// shouldSecureOnLink reports whether a link should turn HTTPS on because the
// project asks for it. A link only ever turns HTTPS on: it is turned off with
// `lerd unsecure`, never as a side effect of linking. secured is a plain bool,
// so an absent .lerd.yaml, an empty one, and one that omits the field all read
// as false — and treating that as "turn HTTPS off" silently dropped a secured
// site back to HTTP every time it was re-linked, undoing the carry-over
// CleanupRelink had just performed. The DNS gate is folded in so a secured:
// true project on a localhost install stays on http rather than triggering a
// runSecure the cert layer would only reject.
func shouldSecureOnLink(projSecured, siteSecured, dnsManaged bool) bool {
	return projSecured && !siteSecured && dnsManaged
}

// summaryEnvReader reads the site's live env file, resolved through the
// framework's env config so a project in a non-dotenv format is parsed in its
// own. Deliberately not sailReadRawEnv, which prefers the .env.before_lerd
// backup the first link has just written, so the summary reported the database
// lerd had replaced rather than the one it configured (#1144).
func summaryEnvReader(site config.Site) func(key string) string {
	envPath := filepath.Join(site.Path, ".env")
	format := "dotenv"
	if fw, ok := config.GetFrameworkForDir(site.Framework, site.Path); ok {
		file, f := fw.Env.Resolve(site.Path)
		envPath = filepath.Join(site.Path, file)
		format = f
	}
	return makeEnvReader(envExampleFallback(envPath), format)
}

// linkRuntimeLabel names the serving runtime for the link summary's PHP row.
func linkRuntimeLabel(site config.Site) string {
	switch {
	case site.IsFrankenPHP():
		return "FrankenPHP"
	case site.IsCustomFPM():
		return "custom FPM"
	default:
		return "FPM"
	}
}

// ensureRequiredServices folds the framework's required service presets into the
// project's service list, so linkApplyServices installs and starts them like any
// other declared service and a teammate cloning the repo gets them too. A name
// the service store does not know is reported and skipped rather than written
// into the project's committed config. resolvePreset is a seam for tests; the
// real one fetches a store-only preset, since a required service is precisely
// the one that is not installed yet.
func ensureRequiredServices(cwd string, proj *config.ProjectConfig, fw *config.Framework, resolvePreset func(string) bool) *config.ProjectConfig {
	if fw == nil || len(fw.Requires) == 0 {
		return proj
	}
	if proj == nil {
		proj = &config.ProjectConfig{}
	}
	have := make(map[string]bool, len(proj.Services))
	for _, s := range proj.Services {
		have[s.Name] = true
	}
	var added []config.ProjectService
	for _, name := range fw.Requires {
		if have[name] {
			continue
		}
		if !resolvePreset(name) {
			feedback.Warn("%s requires the %q service, which the service store does not have", frameworkLabelOf(fw), name)
			continue
		}
		svc := config.ProjectService{Name: name}
		if !config.IsDefaultPreset(name) {
			svc.Preset = name
		}
		proj.Services = append(proj.Services, svc)
		have[name] = true
		added = append(added, svc)
	}
	// Append through a fresh read-modify-write: proj was loaded before this
	// command wrote the domains and the framework version, so saving it whole
	// would roll both back.
	if err := config.AddProjectServices(cwd, added); err != nil {
		feedback.Warn("could not save .lerd.yaml: %v", err)
	}
	return proj
}

// presetResolvable reports whether a preset name can be served, fetching a
// store-only preset into the local cache on the way. PresetExists alone would
// reject any preset not already installed, which is the whole point of requires.
func presetResolvable(name string) bool {
	if config.PresetExists(name) {
		return true
	}
	_, err := config.EnsurePreset(name)
	return err == nil
}

// frameworkLabelOf prefers the display label, falling back to the slug.
func frameworkLabelOf(fw *config.Framework) string {
	if fw.Label != "" {
		return fw.Label
	}
	return fw.Name
}

// linkApplyServices installs and starts services declared in .lerd.yaml.
// Shared by both the standard PHP link path and the custom container path.
// approveInlineService surfaces a brand-new inline service defined in a
// project's .lerd.yaml and confirms it before lerd installs and runs it as a
// container, since the image and command come from the (possibly cloned) repo.
// A scripted or UI link (--yes) and a non-interactive run proceed; an
// interactive run prompts.
func approveInlineService(svc *config.CustomService) bool {
	if linkAssumeYes || !isInteractive() {
		return true
	}
	fmt.Printf("\nThis project defines a service lerd will run as a container:\n")
	fmt.Printf("  name:  %s\n", svc.Name)
	fmt.Printf("  image: %s\n", svc.Image)
	if svc.Exec != "" {
		fmt.Printf("  exec:  %s\n", svc.Exec)
	}
	if len(svc.Ports) > 0 {
		fmt.Printf("  ports: %s\n", strings.Join(svc.Ports, ", "))
	}
	return promptConfirm("Install and start it?")
}

func linkApplyServices(cwd string, proj *config.ProjectConfig) error {
	if proj == nil {
		return nil
	}
	for _, svc := range proj.Services {
		// SQLite is a per-project file, not a container: env setup writes its vars
		// and creates the db file. There is nothing to install or start here.
		if svc.Name == "sqlite" {
			continue
		}
		// A bare entry whose name is a bundled tool preset (e.g. phpmyadmin from a
		// detected docker-compose, or an older .lerd.yaml written before this was
		// normalised) has no Preset/Custom set; resolve it to its preset so it
		// installs instead of failing as a missing custom service.
		if svc.Preset == "" && svc.Custom == nil && config.PresetExists(svc.Name) && !config.IsDefaultPreset(svc.Name) {
			svc.Preset = svc.Name
		}
		if svc.Preset != "" {
			if _, err := config.LoadCustomService(svc.Name); err != nil {
				fmt.Printf("  Installing preset %s%s\n", svc.Preset, presetVersionSuffix(svc.PresetVersion))
				if _, err := InstallPresetByName(svc.Preset, svc.PresetVersion); err != nil {
					feedback.Warn("installing preset %s: %v", svc.Preset, err)
					continue
				}
			}
		} else if svc.Custom != nil {
			svc.Custom.Name = svc.Name
			existing, loadErr := config.LoadCustomService(svc.Name)
			shouldSave := true
			if loadErr != nil {
				// Brand-new inline service from the project's .lerd.yaml: its
				// image and command come from the (possibly cloned) repo, so
				// show what it will run and confirm before installing it.
				if !approveInlineService(svc.Custom) {
					fmt.Printf("  Skipped service %s\n", svc.Name)
					continue
				}
			}
			if loadErr == nil {
				action, err := confirmReplace("service", svc.Name, existing, svc.Custom)
				if err != nil {
					return err
				}
				switch action {
				case replaceFromProject:
					shouldSave = true
				case replaceFromDisk:
					svc.Custom = existing
					shouldSave = false
					if p, _ := config.LoadProjectConfig(cwd); p != nil {
						for i, s := range p.Services {
							if s.Name == svc.Name {
								p.Services[i].Custom = existing
								_ = config.SaveProjectConfig(cwd, p)
								break
							}
						}
					}
				default:
					shouldSave = false
				}
			}
			if shouldSave {
				if err := config.SaveCustomService(svc.Custom); err != nil {
					feedback.Warn("registering service %s: %v", svc.Name, err)
					continue
				}
			}
		}
		if err := ensureServiceRunning(svc.Name); err != nil {
			feedback.Warn("service %s: %v", svc.Name, err)
		}
	}
	return nil
}

// sailLinkDetectDBName reads DB_DATABASE from the project's .env so the link
// prompt can pass the correct Sail database name directly to runImportSail.
// Returns "" when .env is absent or DB_DATABASE is not set.
func sailLinkDetectDBName(cwd string) string {
	env := sailReadRawEnv(cwd)
	return env["DB_DATABASE"]
}

// startWorkersForSite starts the named workers for a site.
// Workers with a Check rule that doesn't pass are skipped. Workers that conflict
// with another requested worker are resolved via ConflictsWith (e.g. horizon replaces queue).
func startWorkersForSite(site *config.Site, workers []string, phpVersion string) {
	// Stripe is not a framework worker, so the framework loop below skips it.
	// Start its listener directly when it was among the workers being restored
	// (e.g. recreated after a runtime switch), before the no-framework early
	// return so it survives even on a site lerd has no framework definition for.
	for _, w := range workers {
		if w == "stripe" {
			scheme := "http"
			if site.Secured {
				scheme = "https"
			}
			baseURL := scheme + "://" + site.PrimaryDomain()
			if err := StripeStartForSite(site.Name, site.Path, baseURL); err != nil {
				feedback.Warn("starting stripe listener: %v", err)
			}
			break
		}
	}

	fw, hasFw := config.GetFrameworkForDir(site.Framework, site.Path)
	if !hasFw || fw.Workers == nil {
		return
	}

	// Build a set of requested workers, applying conflict resolution.
	// If a worker with ConflictsWith is requested AND its conflicts are also
	// requested, the conflicting workers are removed (e.g. horizon removes queue).
	requested := make(map[string]bool, len(workers))
	for _, w := range workers {
		requested[w] = true
	}

	// Check if any worker with conflicts should replace others.
	for _, w := range workers {
		wDef, ok := fw.Workers[w]
		if !ok {
			continue
		}
		// Skip workers whose check doesn't pass. A host worker (vite et al) the
		// user opted into but whose deps aren't installed would otherwise drop
		// silently, reading as "the worker just isn't running"; surface the
		// reason and remedy instead.
		if wDef.Check != nil && !config.MatchesRule(site.Path, *wDef.Check) {
			delete(requested, w)
			if msg := hostWorkerNotReadyMsg(w, site.Path, wDef); msg != "" {
				feedback.Warn("%s", msg)
			}
			continue
		}
		for _, conflict := range wDef.ConflictsWith {
			delete(requested, conflict)
		}
	}

	for _, w := range workers {
		if !requested[w] {
			continue
		}
		worker, ok := fw.Workers[w]
		if !ok {
			continue
		}
		// Skip workers whose check doesn't pass.
		if worker.Check != nil && !config.MatchesRule(site.Path, *worker.Check) {
			continue
		}
		// Stop conflicting workers before starting.
		for _, conflict := range worker.ConflictsWith {
			WorkerStopForSite(site.Name, site.Path, conflict) //nolint:errcheck
		}
		if err := WorkerStartForSite(site.Name, site.Path, phpVersion, w, worker, true); err != nil {
			feedback.Warn("starting worker %s: %v", w, err)
		}
	}
}

// hasRunningWorkers returns true if any workers are currently active for the site.
func hasRunningWorkers(site *config.Site) bool {
	return len(collectRunningWorkers(site)) > 0
}

// isInteractive returns true if stdin is a terminal.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// linkNextStep returns the guidance shown after a standalone lerd link, and
// whether to show it at all. It always points at lerd setup: setup takes a
// freshly linked project the rest of the way (deps, migrations, workers, HTTPS)
// and runs the init wizard itself when there is no .lerd.yaml, so suggesting
// init separately is redundant friction. Returns suggest=false when link runs
// inside setup/init (skipPrompt), so the guidance never nags mid-setup.
func linkNextStep(skipPrompt bool) (hint string, suggest bool) {
	if skipPrompt {
		return "", false
	}
	return "\nRun 'lerd setup' to install dependencies, run migrations, and start workers.", true
}

// resolveFramework returns the framework name for the project at dir, falling
// back to the interactive store picker for a terminal command.
func resolveFramework(dir string) (string, bool) {
	return linker.ResolveFramework(dir, true)
}

// findOwningWorktree returns the parent site if cwd is one of its worktree
// checkouts. Used to short-circuit runLink so worktrees don't get registered
// as standalone sites.
func findOwningWorktree(cwd string) (*config.Site, string, bool) {
	return linker.OwningWorktree(cwd)
}

// fetchFrameworkFromStore attempts to install a framework definition from the
// store. Returns true if successful.
func fetchFrameworkFromStore(name, dir string) bool {
	client := store.NewClient()
	version := ""
	if idx, err := client.FetchIndex(); err == nil {
		for _, entry := range idx.Frameworks {
			if entry.Name == name {
				version = store.ResolveVersion(dir, entry.Detect, entry.Versions, "")
				break
			}
		}
	}
	fw, err := client.FetchFramework(name, version)
	if err != nil {
		return false
	}
	if err := config.SaveStoreFramework(fw); err != nil {
		return false
	}
	v := fw.Version
	if v == "" {
		v = "latest"
	}
	fmt.Printf("  Installed %s@%s from store\n", name, v)
	return true
}
