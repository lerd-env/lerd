package linker

import (
	"fmt"
	"time"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/siteops"
)

// Deps are the side effects a link needs that live above this package. A nil
// field means the caller cannot do that thing, and the step is skipped.
type Deps struct {
	// EnsureFPMQuadlet builds a PHP version's image and unit.
	EnsureFPMQuadlet func(version string) error
	// ReconcileRuntimeQuadlets clears a per-site FrankenPHP or custom-FPM unit
	// the site no longer uses.
	ReconcileRuntimeQuadlets func(site config.Site)
	// StartHostProxyWorker supervises a host-proxy site's dev command.
	StartHostProxyWorker func(site config.Site, proxy *config.ProxyConfig)
	// SyncIDEDataSource points a JetBrains project at the site's database and
	// reports whether it wrote anything.
	SyncIDEDataSource func(siteRoot string) bool
}

// Result is what a link did.
type Result struct {
	Plan *Plan
	// Site is the registration as written, which can differ from the plan's
	// when a newly installed PHP version was accepted.
	Site config.Site
	// WroteIDEDataSource records that a JetBrains data source was written.
	WroteIDEDataSource bool
}

// Registered reports whether a site was actually registered.
func (r *Result) Registered() bool { return r.Plan != nil && r.Plan.Registered() }

// Apply carries out a plan: it registers the site, provisions the runtime that
// serves it, and performs the project writes the policy allows. A plan that
// skips registers nothing and returns without error.
func Apply(plan *Plan, p Policy, d Deps, r Reporter) (*Result, error) {
	if r == nil {
		r = NopReporter{}
	}
	res := &Result{Plan: plan, Site: plan.Site}
	if !plan.Registered() {
		return res, nil
	}

	for _, dropped := range plan.DroppedDomains {
		if existing, err := config.IsDomainUsed(dropped); err == nil && existing != nil {
			r.Warn("domain %q already used by site %q — skipped", dropped, existing.Name)
			continue
		}
		r.Warn("domain %q is reserved — skipped", dropped)
	}
	if plan.FrankenPHPDeclined {
		r.Line(fmt.Sprintf("FrankenPHP has no PHP %s image; linking as FPM instead", plan.Site.PHPVersion))
	}
	if err := offerBetterPHP(plan, p, d, r); err != nil {
		return res, err
	}
	if plan.Mode == ModeHostProxy {
		if err := gateProxyCommand(plan, p, r); err != nil {
			return res, err
		}
	}

	// Carry a re-link's HTTPS over and drop registrations the rename stranded.
	// Resolve already read the secured half; this is the removal.
	siteops.CleanupRelink(plan.Dir, plan.Site.Name)

	site := plan.Site
	if err := config.AddSite(site); err != nil {
		return res, fmt.Errorf("registering site: %w", err)
	}
	res.Site = site

	if d.ReconcileRuntimeQuadlets != nil {
		d.ReconcileRuntimeQuadlets(site)
	}
	if p.ProjectWrites {
		cfg, _ := config.LoadGlobal()
		_ = config.SyncProjectDomains(plan.Dir, site.Domains, cfg.DNS.TLD)
		// A custom-FPM site takes its version from the Containerfile and a
		// proxied site has none, so neither has a version the file should pin.
		if !site.IsCustomContainer() && !site.IsHostProxy() {
			_ = config.SyncProjectFrameworkVersion(site.Framework, plan.Dir)
			_ = siteops.PinPHPVersionFile(plan.Dir, site.PHPVersion)
		}
	}

	if err := provision(plan, site, p, r); err != nil {
		return res, err
	}

	if plan.Mode == ModeHostProxy && p.RepoCommands && d.StartHostProxyWorker != nil {
		d.StartHostProxyWorker(site, plan.Project.Proxy)
	}
	if d.SyncIDEDataSource != nil {
		res.WroteIDEDataSource = d.SyncIDEDataSource(site.Path)
	}
	return res, nil
}

// provision writes the vhost and runtime units for the site's mode, then issues
// its certificate. The certificate is a separate step from the runtime so
// mkcert's work gets its own line rather than being buried in the runtime's.
func provision(plan *Plan, site config.Site, p Policy, r Reporter) error {
	label, detail := provisionLabels(plan, site, r)

	unsecured := site
	unsecured.Secured = false
	// A proxied site has no runtime to provision, so its finisher writes the
	// vhost without a step of its own.
	if label == "" {
		if err := finish(plan, unsecured, p); err != nil {
			return err
		}
	} else {
		step := r.Step(label)
		if err := finish(plan, unsecured, p); err != nil {
			step.Fail(err)
			return err
		}
		step.OK(detail)
	}

	if !site.Secured {
		return nil
	}
	cert := r.Step("generating certificate")
	if err := certs.SecureSite(site); err != nil {
		cert.Fail(err)
		return err
	}
	// SecureSite swaps the vhost to HTTPS on disk without reloading nginx, and
	// the finisher above only reloaded the plain-HTTP config, so without this
	// the site keeps serving the deleted HTTP vhost from nginx's memory. Retry:
	// a concurrent cert reissue can briefly race the swap.
	if err := nginx.ReloadWithRetry(10 * time.Second); err != nil {
		cert.Fail(err)
		return err
	}
	cert.OK("trusted")
	return nil
}

// finish runs the siteops finisher for the plan's serving mode. Only the shared
// FPM runtime can defer its publish; the other modes provision a per-site
// container, which a batch has no way to hoist.
func finish(plan *Plan, site config.Site, p Policy) error {
	switch plan.Mode {
	case ModeCustomContainer:
		return siteops.FinishCustomLink(site, plan.Project.Container)
	case ModeHostProxy:
		return siteops.FinishHostProxyLink(site)
	case ModeCustomFPM:
		return siteops.FinishCustomFPMLink(site, plan.Project.Container)
	case ModeFrankenPHP:
		return siteops.FinishFrankenPHPLink(site)
	default:
		if p.DeferPublish {
			return siteops.FinishSiteOnly(site, site.PHPVersion)
		}
		return siteops.FinishLink(site, site.PHPVersion)
	}
}

// provisionLabels names the provisioning step and its result for a mode.
func provisionLabels(plan *Plan, site config.Site, r Reporter) (label, detail string) {
	switch plan.Mode {
	case ModeCustomContainer, ModeHostProxy:
		return "", ""
	case ModeCustomFPM:
		return "building custom FPM image", "php " + r.Val(site.PHPVersion) + " · nginx vhost written"
	case ModeFrankenPHP:
		return "provisioning FrankenPHP runtime", phpNodeDetail(site, r)
	default:
		return "provisioning PHP-FPM runtime", phpNodeDetail(site, r)
	}
}

// offerBetterPHP offers to install a PHP version that suits the framework
// better than the one the plan settled on, and adopts it when the offer is
// taken. A caller with no prompt reports the clamp instead of asking.
func offerBetterPHP(plan *Plan, p Policy, d Deps, r Reporter) error {
	if plan.PHPMin == "" && plan.PHPMax == "" {
		return nil
	}
	if plan.PHPSuggestion == "" || p.Prompt == nil || d.EnsureFPMQuadlet == nil {
		r.Line(fmt.Sprintf("Using PHP %s (%s supports %s–%s)",
			plan.Site.PHPVersion, plan.FrameworkLabel, plan.PHPMin, plan.PHPMax))
		return nil
	}

	q := fmt.Sprintf("Using PHP %s (best installed in range %s–%s). Install PHP %s?",
		plan.Site.PHPVersion, plan.PHPMin, plan.PHPMax, plan.PHPSuggestion)
	if !p.Prompt.Confirm(q, true) {
		return nil
	}
	step := r.Step("installing PHP " + plan.PHPSuggestion)
	if err := d.EnsureFPMQuadlet(plan.PHPSuggestion); err != nil {
		step.Fail(err)
		return nil // the site links on the version it already has
	}
	step.OK("")
	plan.Site.PHPVersion = plan.PHPSuggestion
	return nil
}

// gateProxyCommand enforces consent before lerd supervises a repository's dev
// command on the host. A policy that forbids repo commands refuses outright; a
// caller with a prompt asks; anything else needs prior approval, --yes, or the
// global skip switch.
func gateProxyCommand(plan *Plan, p Policy, r Reporter) error {
	command := plan.ProxyCommand
	if command == "" {
		return nil // proxy-only: lerd supervises nothing
	}
	if !p.RepoCommands {
		r.Warn("not starting the dev command for %s: this context does not run repository commands", plan.Site.Name)
		plan.Site.HostCommand = ""
		return nil
	}

	gcfg, _ := config.LoadGlobal()
	approved := p.AssumeYes
	known := false
	if existing, err := config.FindSite(plan.Site.Name); err == nil && existing.HostCommand == command {
		approved, known = true, true
	}
	proceed, ask, reason := HostProxyGate(command, gcfg.HostProxy.Disabled, gcfg.HostProxy.SkipConfirmation, approved, p.Prompt != nil)
	if proceed {
		// Consent given out of band (--yes, or a click in the dashboard) approves
		// a command the user has not been shown. Name it, so the thing lerd is
		// about to run on the host appears in the output either way.
		if !known && command != "" {
			r.Line(fmt.Sprintf("supervising this dev-server command on your host, outside any container:\n\n  %s", command))
		}
		return nil
	}
	if !ask {
		return fmt.Errorf("host-proxy %s: %s", plan.Site.Name, reason)
	}
	r.Line(fmt.Sprintf("lerd supervises this dev-server command on your host, outside any container:\n\n  %s", command))
	if !p.Prompt.Confirm(fmt.Sprintf("Start and auto-restart it for %s?", plan.Site.Name), false) {
		return fmt.Errorf("host-proxy setup declined for %s", plan.Site.Name)
	}
	return nil
}

// phpNodeDetail is the result line shared by the two PHP runtimes.
func phpNodeDetail(site config.Site, r Reporter) string {
	return "php " + r.Val(site.PHPVersion) + " · node " + r.Val(site.NodeVersion) + " · nginx vhost written"
}
