package serviceops

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

type domainCalls struct {
	certs       []string
	removedCert []string
	vhosts      []string
	removed     []string
	hosts       int
	reloads     int
}

func stubDomainSeams(t *testing.T) *domainCalls {
	t.Helper()
	rec := &domainCalls{}
	prevCert, prevVhost, prevRemove, prevHosts, prevReload, prevRemoveCert :=
		domainIssueCertFn, domainVhostFn, domainRemoveVhostFn, domainWriteHostsFn, domainReloadFn, domainRemoveCertFn
	domainRemoveCertFn = func(domain string) { rec.removedCert = append(rec.removedCert, domain) }
	domainIssueCertFn = func(domain string) error {
		rec.certs = append(rec.certs, domain)
		return nil
	}
	domainVhostFn = func(domain, upstreamHost string, upstreamPort int, ssl bool) error {
		rec.vhosts = append(rec.vhosts, fmt.Sprint(domain, "|", upstreamHost, "|", upstreamPort, "|", ssl))
		return nil
	}
	domainRemoveVhostFn = func(domain string) error {
		rec.removed = append(rec.removed, domain)
		return nil
	}
	domainWriteHostsFn = func() error { rec.hosts++; return nil }
	prevWait := domainWaitServedFn
	domainWaitServedFn = func(string, time.Duration) bool { return true }
	domainReloadFn = func() error { rec.reloads++; return nil }
	t.Cleanup(func() {
		domainIssueCertFn, domainVhostFn, domainRemoveVhostFn, domainWriteHostsFn, domainReloadFn, domainRemoveCertFn =
			prevCert, prevVhost, prevRemove, prevHosts, prevReload, prevRemoveCert
		domainWaitServedFn = prevWait
	})
	return rec
}

func domainTestConfig(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	sc := cfg.Services["rustfs"]
	sc.Port = 9000
	cfg.Services["rustfs"] = sc
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
}

// A bare name is the common way to type one, and lerd resolves exactly one TLD,
// so the qualification has to happen here rather than becoming a name that
// answers nowhere.
func TestValidateServiceDomain_QualifiesABareName(t *testing.T) {
	domainTestConfig(t)
	got, err := ValidateServiceDomain("rustfs", "storage")
	if err != nil {
		t.Fatalf("ValidateServiceDomain: %v", err)
	}
	if got != "storage.test" {
		t.Errorf("domain = %q, want storage.test", got)
	}
}

func TestValidateServiceDomain_RejectsAForeignTLD(t *testing.T) {
	domainTestConfig(t)
	if _, err := ValidateServiceDomain("rustfs", "storage.local"); err == nil {
		t.Fatal("expected a foreign TLD to be rejected")
	}
}

func TestValidateServiceDomain_RejectsADomainASiteOwns(t *testing.T) {
	domainTestConfig(t)
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir()}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	_, err := ValidateServiceDomain("rustfs", "shop.test")
	if err == nil || !strings.Contains(err.Error(), "shop") {
		t.Fatalf("expected the site collision to be reported, got %v", err)
	}
}

func TestSetServiceDomain_PersistsAndServesIt(t *testing.T) {
	domainTestConfig(t)
	rec := stubDomainSeams(t)

	domain, err := SetServiceDomain("rustfs", "rustfs", 0)
	if err != nil {
		t.Fatalf("SetServiceDomain: %v", err)
	}
	if domain != "rustfs.test" {
		t.Errorf("domain = %q, want rustfs.test", domain)
	}
	if config.ServiceDomain("rustfs") != "rustfs.test" {
		t.Errorf("domain not persisted, got %q", config.ServiceDomain("rustfs"))
	}
	if len(rec.certs) != 1 || rec.certs[0] != "rustfs.test" {
		t.Errorf("expected a certificate for rustfs.test, got %v", rec.certs)
	}
	// nginx reaches the service across the podman network, so the vhost has to
	// point at the container port rather than the published host one.
	if len(rec.vhosts) != 1 || rec.vhosts[0] != "rustfs.test|lerd-rustfs|9000|true" {
		t.Errorf("unexpected vhost: %v", rec.vhosts)
	}
	if rec.hosts == 0 || rec.reloads == 0 {
		t.Errorf("expected the hosts file and nginx to be refreshed, got hosts=%d reloads=%d", rec.hosts, rec.reloads)
	}
}

// A renamed domain leaves the old vhost answering otherwise, and whoever still
// points at the old name keeps being served by it.
func TestSetServiceDomain_RenameDropsTheOldVhost(t *testing.T) {
	domainTestConfig(t)
	rec := stubDomainSeams(t)

	if _, err := SetServiceDomain("rustfs", "rustfs", 0); err != nil {
		t.Fatalf("first SetServiceDomain: %v", err)
	}
	if _, err := SetServiceDomain("rustfs", "storage", 0); err != nil {
		t.Fatalf("second SetServiceDomain: %v", err)
	}
	if len(rec.removed) != 1 || rec.removed[0] != "rustfs.test" {
		t.Errorf("expected the old vhost to be removed, got %v", rec.removed)
	}
}

func TestRemoveServiceDomain_ClearsConfigAndVhost(t *testing.T) {
	domainTestConfig(t)
	rec := stubDomainSeams(t)

	if _, err := SetServiceDomain("rustfs", "rustfs", 0); err != nil {
		t.Fatalf("SetServiceDomain: %v", err)
	}
	if err := RemoveServiceDomain("rustfs"); err != nil {
		t.Fatalf("RemoveServiceDomain: %v", err)
	}
	if config.ServiceDomain("rustfs") != "" {
		t.Errorf("domain still set: %q", config.ServiceDomain("rustfs"))
	}
	if len(rec.removed) != 1 || rec.removed[0] != "rustfs.test" {
		t.Errorf("expected the vhost to be removed, got %v", rec.removed)
	}
	// A key left behind for a host nothing serves is the same wart unlinking a
	// site avoids by dropping its certificate with it.
	if len(rec.removedCert) != 1 || rec.removedCert[0] != "rustfs.test" {
		t.Errorf("expected the certificate to be removed, got %v", rec.removedCert)
	}
}

func TestApplyServiceDomain_NoDomainIsANoOp(t *testing.T) {
	domainTestConfig(t)
	rec := stubDomainSeams(t)

	if err := ApplyServiceDomain("rustfs"); err != nil {
		t.Fatalf("ApplyServiceDomain: %v", err)
	}
	if len(rec.certs) != 0 || len(rec.vhosts) != 0 {
		t.Errorf("expected nothing to be written, got certs=%v vhosts=%v", rec.certs, rec.vhosts)
	}
}

func TestDefaultServiceDomain_UsesTheConfiguredTLD(t *testing.T) {
	domainTestConfig(t)
	if got := DefaultServiceDomain("rustfs"); got != "rustfs.test" {
		t.Errorf("DefaultServiceDomain = %q, want rustfs.test", got)
	}
}

// The fix has to arrive with the update. A service whose preset declares a
// domain and whose user never chose one takes it on the next reconcile.
func TestAdoptDefaultServiceDomains_TakesThePresetDefault(t *testing.T) {
	domainTestConfig(t)
	rec := stubDomainSeams(t)
	prev := UnitInstalledFn
	UnitInstalledFn = func(string) bool { return true }
	t.Cleanup(func() { UnitInstalledFn = prev })

	adopted := AdoptDefaultServiceDomains()

	if len(adopted) != 1 || adopted[0] != "rustfs" {
		t.Fatalf("expected rustfs to adopt its preset domain, got %v", adopted)
	}
	if got := config.ServiceDomain("rustfs"); got != "rustfs.test" {
		t.Errorf("domain = %q, want rustfs.test", got)
	}
	if len(rec.vhosts) != 1 {
		t.Errorf("expected the domain to be served, got %v", rec.vhosts)
	}
}

// Taking the domain away has to stick, or the next start hands it straight back
// and the removal reads as broken.
func TestAdoptDefaultServiceDomains_RespectsAnExplicitRemoval(t *testing.T) {
	domainTestConfig(t)
	stubDomainSeams(t)
	prev := UnitInstalledFn
	UnitInstalledFn = func(string) bool { return true }
	t.Cleanup(func() { UnitInstalledFn = prev })

	if _, err := SetServiceDomain("rustfs", "rustfs", 0); err != nil {
		t.Fatalf("SetServiceDomain: %v", err)
	}
	if err := RemoveServiceDomain("rustfs"); err != nil {
		t.Fatalf("RemoveServiceDomain: %v", err)
	}

	if adopted := AdoptDefaultServiceDomains(); len(adopted) != 0 {
		t.Fatalf("a removed domain must not be handed back, got %v", adopted)
	}
	if got := config.ServiceDomain("rustfs"); got != "" {
		t.Errorf("domain = %q, want it to stay removed", got)
	}
}

// A service that is not installed has nothing to serve on the name.
func TestAdoptDefaultServiceDomains_SkipsAnUninstalledService(t *testing.T) {
	domainTestConfig(t)
	stubDomainSeams(t)
	prev := UnitInstalledFn
	UnitInstalledFn = func(string) bool { return false }
	t.Cleanup(func() { UnitInstalledFn = prev })

	if adopted := AdoptDefaultServiceDomains(); len(adopted) != 0 {
		t.Fatalf("expected nothing adopted for an uninstalled service, got %v", adopted)
	}
}

// Choosing a domain by hand is the opposite of turning it off, so it has to
// clear the record that keeps the default away.
func TestSetServiceDomain_ClearsAnEarlierOptOut(t *testing.T) {
	domainTestConfig(t)
	stubDomainSeams(t)
	prev := UnitInstalledFn
	UnitInstalledFn = func(string) bool { return true }
	t.Cleanup(func() { UnitInstalledFn = prev })

	if _, err := SetServiceDomain("rustfs", "rustfs", 0); err != nil {
		t.Fatalf("SetServiceDomain: %v", err)
	}
	if err := RemoveServiceDomain("rustfs"); err != nil {
		t.Fatalf("RemoveServiceDomain: %v", err)
	}
	if _, err := SetServiceDomain("rustfs", "storage", 0); err != nil {
		t.Fatalf("second SetServiceDomain: %v", err)
	}
	if config.ServiceConfigFor("rustfs").DomainOptOut {
		t.Error("choosing a domain should clear the opt-out")
	}
}

// Removing the service is not a statement about domains. Recording a refusal
// there would leave a reinstall silently without the default its preset ships.
func TestClearServiceDomain_DoesNotRecordARefusal(t *testing.T) {
	domainTestConfig(t)
	stubDomainSeams(t)
	prev := UnitInstalledFn
	UnitInstalledFn = func(string) bool { return true }
	t.Cleanup(func() { UnitInstalledFn = prev })

	if _, err := SetServiceDomain("rustfs", "rustfs", 0); err != nil {
		t.Fatalf("SetServiceDomain: %v", err)
	}
	if err := ClearServiceDomain("rustfs"); err != nil {
		t.Fatalf("ClearServiceDomain: %v", err)
	}
	if config.ServiceConfigFor("rustfs").DomainOptOut {
		t.Fatal("removing the service must not count as refusing the domain")
	}
	if adopted := AdoptDefaultServiceDomains(); len(adopted) != 1 {
		t.Errorf("expected the default to come back on reinstall, got %v", adopted)
	}
}

// The global config entry for a default preset is seeded later than the install
// that adopts its domain, so resolving the port only from config left the vhost
// unwritten and the domain answering nowhere until the user set it by hand.
func TestServiceDomainPort_FallsBackToThePreset(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	cfg.Services["rustfs"] = config.ServiceConfig{}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	if got := serviceDomainPort("rustfs"); got != 9000 {
		t.Errorf("port = %d, want 9000 from the preset", got)
	}
}

// A multi-port service says which port its domain serves, because guessing the
// first mapping lands on SMTP for a mail catcher whose web UI is the point.
func TestServiceDomainPort_PrefersThePresetDeclaration(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	// The primary port is the SMTP one, which is what the old resolver returned.
	cfg.Services["mailpit"] = config.ServiceConfig{Port: 1025}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if got := serviceDomainPort("mailpit"); got != 8025 {
		t.Errorf("port = %d, want the preset's 8025", got)
	}
}

// An explicit choice outranks the preset, for a service whose useful port only
// the user knows.
func TestServiceDomainPort_UserChoiceWins(t *testing.T) {
	domainTestConfig(t)
	stubDomainSeams(t)

	if _, err := SetServiceDomain("rustfs", "console.rustfs.test", 9001); err != nil {
		t.Fatalf("SetServiceDomain: %v", err)
	}
	if got := serviceDomainPort("rustfs"); got != 9001 {
		t.Errorf("port = %d, want the requested 9001", got)
	}
}

// A port nothing listens on writes a vhost that answers nothing, which reads as
// the domain simply not working rather than as the typo it is.
func TestSetServiceDomain_RejectsAPortTheServiceDoesNotExpose(t *testing.T) {
	domainTestConfig(t)
	stubDomainSeams(t)

	_, err := SetServiceDomain("rustfs", "rustfs.test", 1234)
	if err == nil {
		t.Fatal("expected a port the service does not expose to be refused")
	}
	if !strings.Contains(err.Error(), "9000") {
		t.Errorf("the error should name the ports it does expose, got %v", err)
	}
}
