package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/grouping"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// NewApplyCmd returns the apply command: reconcile the machine's sites and
// services against a declarative lerdstead.yml.
func NewApplyCmd() *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "apply [file]",
		Short: "Reconcile sites and services from lerdstead.yml",
		Long: "Link, update, and prune sites declared in a lerdstead.yml file (default " +
			config.LerdsteadFile() + "). Sites in the file are linked or converged to the " +
			"declared domains, PHP version, HTTPS state, and services; sites the file " +
			"previously provisioned and no longer lists are unlinked. Manually linked " +
			"sites are never pruned.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			file := config.LerdsteadFile()
			if len(args) > 0 {
				file = args[0]
			}
			return runApply(file, assumeYes)
		},
	}
	cmd.Flags().BoolVar(&assumeYes, "yes", false, "Unlink sites removed from the file without the confirmation prompt")
	return cmd
}

func runApply(file string, assumeYes bool) error {
	ls, err := config.LoadLerdstead(file)
	if os.IsNotExist(err) {
		return fmt.Errorf("%s not found — create it with a sites: list, or pass a path to lerd apply", file)
	}
	if err != nil {
		return err
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	feedback.Begin()
	feedback.Line("applying " + feedback.Val(file))

	for _, dir := range ls.Park {
		if err := runPark(nil, []string{dir}); err != nil {
			feedback.Warn("parking %s: %v", dir, err)
		}
	}

	for _, ref := range ls.Services {
		ensureSteadService(ref)
	}

	// Suppress the interactive follow-ups a standalone `lerd link` offers;
	// apply runs the pipeline once per declared site and must not stop to chat.
	prevSetup, prevImport := linkSkipSetupPrompt, linkSkipDataImport
	linkSkipSetupPrompt, linkSkipDataImport = true, true
	defer func() { linkSkipSetupPrompt, linkSkipDataImport = prevSetup, prevImport }()

	failed := 0
	for _, entry := range ls.Sites {
		if err := applySteadSite(entry, cfg); err != nil {
			feedback.Warn("%s: %v", entry.Path, err)
			failed++
		}
	}

	if err := pruneSteadSites(ls, assumeYes); err != nil {
		return err
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d site(s) could not be applied", failed, len(ls.Sites))
	}
	feedback.Done(fmt.Sprintf("%d site(s) in sync with %s", len(ls.Sites), file))
	return nil
}

// applySteadSite converges one declared site: link it if unknown, then bring
// domains, PHP version, HTTPS, and services to the declared state. Every step
// is a no-op when the site already matches, so re-applying is free.
func applySteadSite(entry config.LerdsteadSite, cfg *config.GlobalConfig) error {
	path := config.CanonicalPath(entry.Path)
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return fmt.Errorf("directory does not exist")
	}

	site, _ := config.FindSiteByPath(path)
	if site == nil {
		if err := steadLinkSite(path, entry); err != nil {
			return err
		}
		site, _ = config.FindSiteByPath(path)
		if site == nil {
			// runLink declined without an error: a worktree of another site, or
			// a directory it could not register. Nothing to converge.
			return fmt.Errorf("not linkable as a standalone site")
		}
	}

	if !site.Lerdstead {
		site.Lerdstead = true
		if err := config.AddSite(*site); err != nil {
			return err
		}
	}

	if err := steadApplyDomains(site, entry, cfg); err != nil {
		return err
	}
	if err := steadApplyPHP(site, entry); err != nil {
		return err
	}
	if err := steadApplySecured(site, entry, cfg); err != nil {
		return err
	}
	return steadApplyServices(site.Path, entry.Services)
}

// steadLinkSite runs the standard link pipeline for a declared site. runLink
// works off the working directory (it is what `lerd link` and the init wizard
// share), so apply visits the project the same way a user would.
func steadLinkSite(path string, entry config.LerdsteadSite) error {
	prev, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(path); err != nil {
		return err
	}
	defer os.Chdir(prev) //nolint:errcheck

	var args []string
	if len(entry.Domains) > 0 {
		args = []string{entry.Domains[0]}
	}
	return runLink(args)
}

// steadWantDomains maps declared TLD-less domains to full lowercase domains.
func steadWantDomains(domains []string, tld string) []string {
	if len(domains) == 0 {
		return nil
	}
	out := make([]string, len(domains))
	for i, d := range domains {
		out[i] = strings.ToLower(d) + "." + tld
	}
	return out
}

// steadApplyDomains converges the site's domain list to the declared one,
// following the same sequence as `lerd domain add`: registry, .lerd.yaml,
// vhost, cert SANs, container hosts, nginx reload, env, and group cascade.
// A domain owned by another site is skipped with a warning, never stolen.
func steadApplyDomains(site *config.Site, entry config.LerdsteadSite, cfg *config.GlobalConfig) error {
	want := steadWantDomains(entry.Domains, cfg.DNS.TLD)
	if want == nil {
		return nil
	}
	kept := want[:0]
	for _, d := range want {
		if other, err := config.IsDomainUsed(d); err == nil && other != nil && other.Name != site.Name {
			feedback.Warn("domain %s already belongs to site %q; skipping it", d, other.Name)
			continue
		}
		if isReservedDomain(d) {
			feedback.Warn("domain %s is reserved for internal Lerd use; skipping it", d)
			continue
		}
		kept = append(kept, d)
	}
	if len(kept) == 0 || slices.Equal(kept, site.Domains) {
		return nil
	}

	oldPrimary := site.PrimaryDomain()
	site.Domains = slices.Clone(kept)
	if err := config.AddSite(*site); err != nil {
		return fmt.Errorf("updating site registry: %w", err)
	}
	_ = config.SyncProjectDomains(site.Path, site.Domains, cfg.DNS.TLD)
	if err := siteops.RegenerateSiteVhost(site, oldPrimary); err != nil {
		return err
	}
	if site.Secured {
		if err := certs.ReissueCertForWorktree(*site); err != nil {
			feedback.Warn("reissuing certificate: %v", err)
		}
	}
	if err := podman.WriteContainerHosts(); err != nil {
		feedback.Warn("updating container hosts file: %v", err)
	}
	nginx.ReloadOrWarn("")
	if err := siteops.SyncEnvIfPrimaryChanged(site, oldPrimary); err != nil {
		feedback.Warn("syncing .env to new primary domain: %v", err)
	}
	if site.IsGroupMain() {
		if err := grouping.CascadeMainDomainChange(site); err != nil {
			feedback.Warn("cascading group domain change: %v", err)
		}
	}
	feedback.Line(site.Name + " domains → " + feedback.Val(strings.Join(site.Domains, ", ")))
	return nil
}

// steadApplyPHP converges the site to the declared PHP version through the
// shared SetSitePHPVersion funnel, building the FPM image afterwards when the
// version has never been built (the funnel itself never builds).
func steadApplyPHP(site *config.Site, entry config.LerdsteadSite) error {
	if entry.PHPVersion == "" || entry.PHPVersion == site.PHPVersion {
		return nil
	}
	if site.IsCustomContainer() || site.IsHostProxy() {
		feedback.Warn("%s: php_version does not apply to a %s site", site.Name, linkRuntimeKind(site))
		return nil
	}
	step := feedback.Start("switching " + site.Name + " to PHP " + entry.PHPVersion)
	res, err := siteops.SetSitePHPVersion(site, entry.PHPVersion, siteops.PHPVersionOpts{})
	if err != nil {
		step.Fail(err)
		return err
	}
	if res.NotInstalled || res.Stale {
		if err := ensureFPMQuadlet(res.Version); err != nil {
			step.Fail(err)
			return err
		}
	}
	label := feedback.Val(res.Version)
	if res.Clamped {
		label += " (clamped from " + res.Requested + ")"
	}
	step.OK(label)
	return nil
}

// linkRuntimeKind names the serving mode for warnings about settings that
// don't apply to it.
func linkRuntimeKind(site *config.Site) string {
	if site.IsCustomContainer() {
		return "custom-container"
	}
	return "host-proxy"
}

// steadApplySecured converges HTTPS to the declared state. An absent secured
// key (nil) leaves the site alone, so a manual `lerd secure` is not undone by
// a file that never mentions it.
func steadApplySecured(site *config.Site, entry config.LerdsteadSite, cfg *config.GlobalConfig) error {
	if entry.Secured == nil || *entry.Secured == site.Secured {
		return nil
	}
	if *entry.Secured && !cfg.DNSManaged() {
		feedback.Warn("%s: cannot enable HTTPS while lerd DNS is disabled", site.Name)
		return nil
	}
	return toggleSecureCmd([]string{site.Name}, *entry.Secured)
}

// steadApplyServices installs and starts the declared service presets through
// the same path a .lerd.yaml services list takes on link. The list is built in
// memory: lerdstead.yml is machine config and must not rewrite the project's
// committed .lerd.yaml.
func steadApplyServices(path string, refs []string) error {
	if len(refs) == 0 {
		return nil
	}
	return linkApplyServices(path, &config.ProjectConfig{Services: steadProjectServices(refs)})
}

// steadProjectServices maps "name" / "name@version" refs to ProjectService
// entries, mirroring how ensureRequiredServices shapes them.
func steadProjectServices(refs []string) []config.ProjectService {
	out := make([]config.ProjectService, 0, len(refs))
	for _, ref := range refs {
		name, version := parseServiceRef(ref)
		svc := config.ProjectService{Name: name, PresetVersion: version}
		if !config.IsDefaultPreset(name) {
			svc.Preset = name
		}
		out = append(out, svc)
	}
	return out
}

// parseServiceRef splits a "name@version" service reference; version is empty
// when no pin is given.
func parseServiceRef(ref string) (name, version string) {
	name, version, _ = strings.Cut(ref, "@")
	return name, version
}

// ensureSteadService installs a global service preset if missing and makes
// sure it is running.
func ensureSteadService(ref string) {
	name, version := parseServiceRef(ref)
	if _, err := config.LoadCustomService(name); err != nil {
		fmt.Printf("  Installing preset %s%s\n", name, presetVersionSuffix(version))
		if _, err := InstallPresetByName(name, version); err != nil {
			feedback.Warn("installing service %s: %v", name, err)
			return
		}
	}
	if err := ensureServiceRunning(name); err != nil {
		feedback.Warn("service %s: %v", name, err)
	}
}

// steadStaleSites returns the sites apply may prune: provisioned from
// lerdstead.yml (the Lerdstead flag), still active, and no longer declared.
func steadStaleSites(sites []config.Site, declared map[string]bool) []config.Site {
	var out []config.Site
	for _, s := range sites {
		if s.Lerdstead && !s.Ignored && !declared[config.CanonicalPath(s.Path)] {
			out = append(out, s)
		}
	}
	return out
}

// pruneSteadSites unlinks sites the file previously provisioned and no longer
// lists. It confirms first; --yes skips the prompt, and a non-interactive run
// without --yes only reports what it would remove.
func pruneSteadSites(ls *config.Lerdstead, assumeYes bool) error {
	reg, err := config.LoadSites()
	if err != nil {
		return err
	}
	declared := make(map[string]bool, len(ls.Sites))
	for _, entry := range ls.Sites {
		declared[config.CanonicalPath(entry.Path)] = true
	}

	stale := steadStaleSites(reg.Sites, declared)
	if len(stale) == 0 {
		return nil
	}
	names := make([]string, len(stale))
	for i, s := range stale {
		names[i] = s.Name
	}

	if !assumeYes {
		if !isInteractive() {
			feedback.Note("no longer in the file (re-run with --yes to unlink): " + strings.Join(names, ", "))
			return nil
		}
		q := fmt.Sprintf("Unlink %d site(s) no longer in the file (%s)?", len(stale), strings.Join(names, ", "))
		if !feedback.Confirm(q, false) {
			return nil
		}
	}
	for _, s := range stale {
		if err := UnlinkSite(s.Name); err != nil {
			feedback.Warn("unlinking %s: %v", s.Name, err)
		}
	}
	return nil
}
