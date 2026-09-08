package serviceops

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/podman"
)

// The seams the domain flow talks to the rest of the machine through, swapped
// in tests so the vhost, the certificate and the hosts file can be asserted
// without an nginx to reload.
var (
	domainIssueCertFn = func(domain string) error {
		return certs.IssueCert(domain, []string{domain}, filepath.Join(config.CertsDir(), "sites"))
	}
	domainVhostFn       = nginx.GenerateServiceProxyVhost
	domainRemoveCertFn  = certs.RemoveSiteCerts
	domainRemoveVhostFn = nginx.RemoveVhost
	domainWriteHostsFn  = podman.WriteContainerHosts
	domainReloadFn      = nginx.Reload
	domainWaitServedFn  = waitDomainServed
)

// hostnameLabel is one label of a domain name: the shape nginx will accept as a
// server_name and mkcert will sign.
var hostnameLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// DefaultServiceDomain is the domain a service takes when the user names none:
// the service's own name under the configured TLD.
func DefaultServiceDomain(name string) string {
	return name + "." + config.EffectiveTLD()
}

// ServiceDomainURL returns the https URL a service is reachable at from both
// the app container and the browser, empty when the service has no domain.
func ServiceDomainURL(name string) string {
	if domain := config.ServiceDomain(name); domain != "" {
		return "https://" + domain
	}
	return ""
}

// ValidateServiceDomain normalises a requested domain and rejects one that
// nginx, mkcert or another site would refuse. A bare name is qualified with the
// configured TLD, so `lerd service domain rustfs storage` means storage.test.
func ValidateServiceDomain(service, domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(strings.Trim(domain, ".")))
	if domain == "" {
		return "", fmt.Errorf("a domain is required")
	}
	tld := config.EffectiveTLD()
	if !strings.Contains(domain, ".") {
		domain += "." + tld
	}
	for _, label := range strings.Split(domain, ".") {
		if !hostnameLabel.MatchString(label) {
			return "", fmt.Errorf("invalid domain %q: use letters, digits and dashes", domain)
		}
	}
	// Only the TLD lerd resolves can work: the name has to answer inside the app
	// container and in the browser, and lerd owns no other suffix on either.
	if !strings.HasSuffix(domain, "."+tld) {
		return "", fmt.Errorf("domain %q must end in .%s, the TLD lerd resolves", domain, tld)
	}
	if site, err := config.IsDomainUsed(domain); err == nil && site != nil {
		return "", fmt.Errorf("domain %q is already used by site %q", domain, site.Name)
	}
	for other, taken := range config.ServiceDomains() {
		if taken == domain && other != service {
			return "", fmt.Errorf("domain %q is already used by service %q", domain, other)
		}
	}
	return domain, nil
}

// SetServiceDomain gives a service a domain and serves it there: the vhost, its
// certificate and the hosts entry the app containers resolve through all follow
// from the one call, so the name works from both sides the moment it returns.
// A port of 0 leaves the choice to the preset, then to the service's primary.
func SetServiceDomain(service, domain string, port int) (string, error) {
	if !config.IsDefaultPreset(service) && !ServiceInstalled(service) {
		return "", fmt.Errorf("%q is not a built-in or installed service", service)
	}
	resolved, err := ValidateServiceDomain(service, domain)
	if err != nil {
		return "", err
	}
	// A port the service does not listen on writes a vhost that answers nothing,
	// which is indistinguishable from the domain not working at all.
	if port > 0 {
		declared := serviceContainerPorts(service)
		if len(declared) > 0 && !slices.Contains(declared, port) {
			return "", fmt.Errorf("service %q does not expose port %d (it exposes %s)",
				service, port, joinPorts(declared))
		}
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return "", err
	}
	svcCfg := cfg.Services[service]
	previous := svcCfg.Domain
	svcCfg.Domain = resolved
	svcCfg.DomainPort = port
	// Choosing a domain is the opposite of turning it off, so it clears the
	// record that would keep the preset's default away.
	svcCfg.DomainOptOut = false
	cfg.Services[service] = svcCfg
	if err := config.SaveGlobal(cfg); err != nil {
		return "", err
	}
	// A renamed domain leaves its old vhost answering otherwise, and the stale
	// server_name would keep winning for whoever still points at it.
	if previous != "" && previous != resolved {
		_ = domainRemoveVhostFn(previous)
	}
	if err := ApplyServiceDomain(service); err != nil {
		return resolved, err
	}
	return resolved, nil
}

// RemoveServiceDomain takes the domain away and stops serving it. The service
// itself is untouched: it stays reachable at lerd-<name> on the podman network.
// The removal is recorded, not just applied: a preset that declares a default
// domain would otherwise hand it straight back on the next reconcile.
func RemoveServiceDomain(service string) error {
	return removeServiceDomain(service, true)
}

// ClearServiceDomain stops serving the domain without recording a refusal. It is
// what removing the service itself uses: the user said nothing about domains,
// they removed a service, and a reinstall that came back without its default
// would be the surprising outcome.
func ClearServiceDomain(service string) error {
	return removeServiceDomain(service, false)
}

func removeServiceDomain(service string, optOut bool) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	svcCfg := cfg.Services[service]
	if svcCfg.Domain == "" {
		return fmt.Errorf("service %q has no domain", service)
	}
	domain := svcCfg.Domain
	svcCfg.Domain = ""
	svcCfg.DomainOptOut = optOut
	cfg.Services[service] = svcCfg
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	if err := domainRemoveVhostFn(domain); err != nil {
		return fmt.Errorf("removing vhost for %s: %w", domain, err)
	}
	// The certificate goes with the vhost, the way unlinking a site drops its
	// own: nothing serves the name any more, so leaving it is a key on disk for
	// a host that answers nowhere.
	domainRemoveCertFn(domain)
	_ = domainWriteHostsFn()
	return reloadNginxIfRunning()
}

// reloadNginxIfRunning treats a stopped nginx as success. The config on disk is
// what matters here; whoever starts nginx next reads it, and failing the command
// would report a domain as unset when it is written and will serve on start.
func reloadNginxIfRunning() error {
	if err := domainReloadFn(); err != nil && !errors.Is(err, nginx.ErrNotRunning) {
		return err
	}
	return nil
}

// ApplyServiceDomain writes everything a service's domain needs to answer:
// its certificate, its vhost, and the hosts entry the app containers read. It
// is idempotent, so the start and install reconciles can call it blindly.
func ApplyServiceDomain(service string) error {
	domain := config.ServiceDomain(service)
	if domain == "" {
		return nil
	}
	port := serviceDomainPort(service)
	if port == 0 {
		return fmt.Errorf("service %q declares no port to proxy to", service)
	}
	if err := domainIssueCertFn(domain); err != nil {
		return fmt.Errorf("issuing certificate for %s: %w", domain, err)
	}
	if err := domainVhostFn(domain, "lerd-"+service, port, true, serviceDomainCORS(service)); err != nil {
		return fmt.Errorf("writing vhost for %s: %w", domain, err)
	}
	if err := domainWriteHostsFn(); err != nil {
		return fmt.Errorf("updating container hosts: %w", err)
	}
	if err := reloadNginxIfRunning(); err != nil {
		return err
	}
	// An nginx reload is asynchronous: the old workers keep answering until they
	// cycle. Returning here without waiting hands back a URL that the very next
	// request cannot reach yet, which reads as the domain simply not working.
	if domainWaitServedFn(domain, domainReadyTimeout) {
		return nil
	}
	// The signal can be lost outright: an install regenerates units and restarts
	// nginx around the same moment, and a reload that lands in that window is
	// accepted by a process that is on its way out. One retry costs a signal and
	// is the difference between a domain that works and one the user has to
	// reload nginx for by hand.
	if err := reloadNginxIfRunning(); err != nil {
		return err
	}
	domainWaitServedFn(domain, domainReadyTimeout)
	return nil
}

// domainReadyTimeout bounds the wait for nginx to start serving a new service
// domain. A reload is quick; this only has to outlast one worker cycle, and a
// vhost that never comes up must report rather than hang the command.
const domainReadyTimeout = 5 * time.Second

// waitDomainServed blocks until the domain answers over TLS, so the caller does
// not return while the previous config is still being served. Any response is
// enough: the service's own status says nothing about whether nginx picked the
// vhost up, and requiring 200 would wait out a service that is merely empty.
func waitDomainServed(domain string, timeout time.Duration) bool {
	if domain == "" {
		return true
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		// The certificate is lerd's own and only the vhost being live is being
		// measured, so it is neither verified nor followed here.
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get("https://" + domain + "/") //nolint:noctx
		if resp != nil {
			_ = resp.Body.Close()
		}
		if err == nil && resp != nil {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// AdoptDefaultServiceDomains gives every installed service the domain its preset
// declares, unless the user already chose one or took it away. It is what makes
// the domain arrive with an update rather than waiting for each user to find a
// command: a service whose URLs reach a browser is broken without one, and the
// preset is where that fact belongs. Returns the services that took a domain.
//
// Idempotent: once adopted the domain is an ordinary explicit value, so later
// runs see it set and do nothing.
func AdoptDefaultServiceDomains() []string {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return nil
	}
	var adopted []string
	for _, name := range config.DefaultPresetNames() {
		svcCfg := cfg.Services[name]
		if svcCfg.Domain != "" || svcCfg.DomainOptOut {
			continue
		}
		if !UnitInstalledFn("lerd-" + name) {
			continue
		}
		preset, perr := config.LoadPreset(name)
		if perr != nil || preset.Domain == "" {
			continue
		}
		// A default that collides with a site or another service is not worth
		// failing a start over; the user can name a free one by hand.
		resolved, verr := ValidateServiceDomain(name, preset.Domain)
		if verr != nil {
			continue
		}
		svcCfg.Domain = resolved
		cfg.Services[name] = svcCfg
		if err := config.SaveGlobal(cfg); err != nil {
			return adopted
		}
		if err := ApplyServiceDomain(name); err != nil {
			continue
		}
		adopted = append(adopted, name)
	}
	return adopted
}

// ApplyServiceDomains reapplies every configured service domain. Called from
// the reconcile paths so a domain survives a machine that came up with no
// vhosts, or a certificate that aged out.
func ApplyServiceDomains() error {
	var failed []string
	for service := range config.ServiceDomains() {
		if err := ApplyServiceDomain(service); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", service, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("applying service domains: %s", strings.Join(failed, "; "))
	}
	return nil
}

// ServiceDomainPort is serviceDomainPort for callers outside this package: the
// port a service's domain actually proxies to, so a surface can show it rather
// than leave the user guessing which of a multi-port service answers.
func ServiceDomainPort(service string) int { return serviceDomainPort(service) }

// ServiceDomainCORS is serviceDomainCORS for callers outside this package.
func ServiceDomainCORS(service string) bool { return serviceDomainCORS(service) }

// serviceDomainCORS reports whether the domain answers the browser preflight.
// The user's own choice wins in either direction, then the preset's, which is
// where a service that a page talks to directly says so. Silence is not a no:
// only DomainCORSOptOut is, which is what keeps a preset default from being
// handed back to a user who turned it off.
func serviceDomainCORS(service string) bool {
	sc := config.ServiceConfigFor(service)
	if sc.DomainCORSOptOut {
		return false
	}
	if sc.DomainCORS {
		return true
	}
	preset, err := config.LoadPreset(service)
	return err == nil && preset != nil && preset.DomainCORS
}

// SetServiceDomainCORS records the user's answer on the preflight and re-renders
// the vhost so it takes effect without a restart. A service with no domain has
// no vhost to carry it, so the choice is refused rather than saved somewhere it
// would silently do nothing.
func SetServiceDomainCORS(service string, enabled bool) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	if cfg.Services[service].Domain == "" {
		return fmt.Errorf("service %q has no domain to answer preflights on", service)
	}
	svcCfg := cfg.Services[service]
	svcCfg.DomainCORS = enabled
	svcCfg.DomainCORSOptOut = !enabled
	cfg.Services[service] = svcCfg
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	return ApplyServiceDomain(service)
}

// serviceDomainPort is the container-internal port nginx proxies the domain to.
// The published host port is deliberately not used: nginx reaches the service
// across the podman network, where the container port is what listens.
//
// The user's own choice wins, then the preset's, then the service's primary
// port. That last fallback is a guess and only right when the first mapping
// happens to be the HTTP one, which is why a multi-port preset says so.
func serviceDomainPort(service string) int {
	sc := config.ServiceConfigFor(service)
	if sc.DomainPort > 0 {
		return sc.DomainPort
	}
	preset, perr := config.LoadPreset(service)
	if perr == nil && preset != nil && preset.DomainPort > 0 {
		return preset.DomainPort
	}
	if sc.Port > 0 {
		return sc.Port
	}
	if svc, err := config.LoadCustomService(service); err == nil && svc != nil {
		if port := config.MappingContainerPort(firstMapping(svc.Ports)); port > 0 {
			return port
		}
	}
	// The global config entry is seeded later than the install that adopts the
	// domain, so at that moment the only place the port exists is the preset.
	if perr == nil && preset != nil {
		return config.MappingContainerPort(firstMapping(preset.Ports))
	}
	return 0
}

// serviceContainerPorts lists every container port a service declares, so a
// requested one can be checked against what actually listens rather than being
// written into a vhost that answers nothing.
func serviceContainerPorts(service string) []int {
	var ports []int
	add := func(p int) {
		if p <= 0 {
			return
		}
		for _, seen := range ports {
			if seen == p {
				return
			}
		}
		ports = append(ports, p)
	}
	if sc := config.ServiceConfigFor(service); sc.Port > 0 {
		add(sc.Port)
		for containerPort := range sc.PublishedPorts {
			add(containerPort)
		}
		for _, mapping := range sc.ExtraPorts {
			add(config.MappingContainerPort(mapping))
		}
	}
	if svc, err := config.LoadCustomService(service); err == nil && svc != nil {
		for _, mapping := range svc.Ports {
			add(config.MappingContainerPort(mapping))
		}
	}
	if preset, err := config.LoadPreset(service); err == nil && preset != nil {
		for _, mapping := range preset.Ports {
			add(config.MappingContainerPort(mapping))
		}
	}
	return ports
}

// joinPorts renders a port list for an error the user has to act on.
func joinPorts(ports []int) string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, strconv.Itoa(p))
	}
	return strings.Join(out, ", ")
}

func firstMapping(ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	return ports[0]
}
