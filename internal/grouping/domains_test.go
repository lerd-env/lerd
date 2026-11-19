package grouping

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// stubDomainChange records each step ApplyDomainChange takes in place of the
// cert, vhost, hosts, nginx and .env work.
func stubDomainChange(t *testing.T, vhostErr, certErr error) *[]string {
	t.Helper()
	var steps []string
	prevVhost, prevCert, prevHosts, prevReload, prevEnv := regenerateVhostFn, reissueCertFn, writeHostsFn, reloadNginxFn, syncEnvFn
	regenerateVhostFn = func(_ *config.Site, old string) error { steps = append(steps, "vhost:"+old); return vhostErr }
	reissueCertFn = func(config.Site) error { steps = append(steps, "cert"); return certErr }
	writeHostsFn = func() error { steps = append(steps, "hosts"); return nil }
	reloadNginxFn = func() error { steps = append(steps, "reload"); return nil }
	syncEnvFn = func(_ *config.Site, old string) error { steps = append(steps, "env:"+old); return nil }
	t.Cleanup(func() {
		regenerateVhostFn, reissueCertFn, writeHostsFn, reloadNginxFn, syncEnvFn = prevVhost, prevCert, prevHosts, prevReload, prevEnv
	})
	return &steps
}

func TestApplyDomainChange_followsTheNewDomainsEverywhere(t *testing.T) {
	setup(t)
	steps := stubDomainChange(t, nil, nil)
	site := &config.Site{Name: "shop", Domains: []string{"store.test", "shop.test"}, Secured: true}
	warnings, err := ApplyDomainChange(site, "shop.test")
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err = %v, warnings = %v", err, warnings)
	}
	if got := strings.Join(*steps, ","); got != "vhost:shop.test,cert,hosts,reload,env:shop.test" {
		t.Errorf("steps = %s", got)
	}
}

func TestApplyDomainChange_stopsWhenTheVhostCannotBeWritten(t *testing.T) {
	setup(t)
	steps := stubDomainChange(t, errors.New("disk full"), nil)
	_, err := ApplyDomainChange(&config.Site{Name: "shop", Domains: []string{"shop.test"}}, "shop.test")
	if err == nil || len(*steps) != 1 {
		t.Fatalf("err = %v, steps = %v", err, *steps)
	}
}

func TestApplyDomainChange_carriesOnPastAFailedCertificate(t *testing.T) {
	setup(t)
	steps := stubDomainChange(t, nil, errors.New("mkcert missing"))
	warnings, err := ApplyDomainChange(&config.Site{Name: "shop", Domains: []string{"shop.test"}, Secured: true}, "shop.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "reissuing certificate") {
		t.Errorf("warnings = %v", warnings)
	}
	if !strings.HasSuffix(strings.Join(*steps, ","), "env:shop.test") {
		t.Errorf("stopped early: %v", *steps)
	}
}

func TestApplyDomainChange_movesAGroupsSecondariesWithTheirMain(t *testing.T) {
	setup(t)
	stubDomainChange(t, nil, nil)
	mustAdd(t, config.Site{Name: "starlane", Domains: []string{"starlane.test"}, Path: "/srv/starlane"})
	mustAdd(t, config.Site{Name: "admin", Domains: []string{"admin.test"}, Path: "/srv/admin"})
	if err := AssignSecondary(reload(t, "starlane"), reload(t, "admin"), "admin", false); err != nil {
		t.Fatal(err)
	}
	main := reload(t, "starlane")
	main.Domains = []string{"astro.test"}
	mustAdd(t, *main)
	if _, err := ApplyDomainChange(reload(t, "starlane"), "starlane.test"); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, "admin").PrimaryDomain(); got != "admin.astro.test" {
		t.Errorf("admin domain = %q, want admin.astro.test", got)
	}
}

// A plain reload signals the master and returns while the old workers still
// serve, so an added domain answers 404 for a moment after it is reported.
func TestApplyDomainChange_settlesTheReload(t *testing.T) {
	src, err := os.ReadFile("domains.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "reloadNginxFn = webserver.ReloadAndSettle\n") {
		t.Error("ApplyDomainChange must reload through webserver.ReloadAndSettle")
	}
}
