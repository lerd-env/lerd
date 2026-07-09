package siteops

import (
	"errors"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
)

// groupSetup stubs the cert/nginx/daemon side effects and forces DNS on, so the
// invariant can be asserted on registry state alone.
func groupSetup(t *testing.T) *secureStubs {
	t.Helper()
	stubs := stubSecureDeps(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	orig := dnsManagedFn
	dnsManagedFn = func() bool { return true }
	t.Cleanup(func() { dnsManagedFn = orig })
	return stubs
}

func mainSite(name string, secured bool) config.Site {
	return config.Site{Name: name, Domains: []string{name + ".test"}, Path: "/srv/" + name, Secured: secured, Group: name}
}

func secondarySite(name, group, label string, secured bool) config.Site {
	return config.Site{
		Name: name, Domains: []string{label + "." + group + ".test"}, Path: "/srv/" + name,
		Secured: secured, Group: group, GroupSubdomain: label,
	}
}

func mustAdd(t *testing.T, s config.Site) {
	t.Helper()
	if err := config.AddSite(s); err != nil {
		t.Fatalf("AddSite(%s): %v", s.Name, err)
	}
}

func reloadSite(t *testing.T, name string) *config.Site {
	t.Helper()
	s, err := config.FindSite(name)
	if err != nil {
		t.Fatalf("FindSite(%s): %v", name, err)
	}
	return s
}

// The bug: a secondary left on plain HTTP under a secured main has no 443
// block, so the main's *.main wildcard answers its subdomain over HTTPS and
// serves the wrong app (issue #811).
func TestEnforceGroupSecondaries_securesHTTPSecondaryUnderSecuredMain(t *testing.T) {
	groupSetup(t)
	mustAdd(t, mainSite("astrolov", true))
	mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", false))

	changed, err := EnforceGroupSecondaries()
	if err != nil {
		t.Fatalf("EnforceGroupSecondaries: %v", err)
	}
	if len(changed) != 1 || changed[0] != "astrolov-2" {
		t.Fatalf("changed = %v, want [astrolov-2]", changed)
	}
	if !reloadSite(t, "astrolov-2").Secured {
		t.Error("secondary should be secured to match its main")
	}
}

func TestEnforceGroupSecondaries_leavesConsistentGroupsAlone(t *testing.T) {
	cases := []struct {
		name             string
		main, secondary  bool
		wantSecuredAfter bool
	}{
		// Already consistent: nothing to do, and no cert churn.
		{"both secured", true, true, true},
		// An unsecured main has no 443 block, so nothing swallows the
		// secondary. A secondary may legitimately be HTTPS on its own.
		{"unsecured main, secured secondary", false, true, true},
		{"both plain http", false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stubs := groupSetup(t)
			mustAdd(t, mainSite("astrolov", c.main))
			mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", c.secondary))

			changed, err := EnforceGroupSecondaries()
			if err != nil {
				t.Fatalf("EnforceGroupSecondaries: %v", err)
			}
			if len(changed) != 0 {
				t.Errorf("changed = %v, want none", changed)
			}
			if stubs.secureCallCount != 0 {
				t.Errorf("certs.SecureSite called %d times, want no cert churn", stubs.secureCallCount)
			}
			if got := reloadSite(t, "astrolov-2"); got.Secured != c.wantSecuredAfter {
				t.Errorf("secondary secured = %v, want %v", got.Secured, c.wantSecuredAfter)
			}
		})
	}
}

// With DNS off there is no HTTPS at all, so every site is plain HTTP and the
// wildcard cannot swallow anything. Enforcing would only fail on ErrDNSDisabled.
func TestEnforceGroupSecondaries_noopWhenDNSDisabled(t *testing.T) {
	stubs := groupSetup(t)
	dnsManagedFn = func() bool { return false }
	mustAdd(t, mainSite("astrolov", true))
	mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", false))

	changed, err := EnforceGroupSecondaries()
	if err != nil {
		t.Fatalf("EnforceGroupSecondaries: %v", err)
	}
	if len(changed) != 0 || stubs.secureCallCount != 0 {
		t.Errorf("changed=%v secureCalls=%d, want no action with DNS disabled", changed, stubs.secureCallCount)
	}
}

// A standalone site that happens to be unsecured must never be touched: only
// secondaries inherit, and only from their own group's main.
func TestEnforceGroupSecondaries_ignoresUngroupedAndOtherGroups(t *testing.T) {
	stubs := groupSetup(t)
	mustAdd(t, config.Site{Name: "solo", Domains: []string{"solo.test"}, Path: "/srv/solo"})
	mustAdd(t, mainSite("astrolov", true))
	mustAdd(t, mainSite("other", false))
	mustAdd(t, secondarySite("other-2", "other", "blog", false))

	changed, err := EnforceGroupSecondaries()
	if err != nil {
		t.Fatalf("EnforceGroupSecondaries: %v", err)
	}
	if len(changed) != 0 || stubs.secureCallCount != 0 {
		t.Errorf("changed=%v secureCalls=%d, want no action", changed, stubs.secureCallCount)
	}
}

// A secondary whose group has no main in the registry has no wildcard above it.
func TestEnforceGroupSecondaries_orphanSecondaryIsLeftAlone(t *testing.T) {
	stubs := groupSetup(t)
	mustAdd(t, secondarySite("orphan", "gone", "blog", false))

	changed, err := EnforceGroupSecondaries()
	if err != nil {
		t.Fatalf("EnforceGroupSecondaries: %v", err)
	}
	if len(changed) != 0 || stubs.secureCallCount != 0 {
		t.Errorf("changed=%v secureCalls=%d, want no action for an orphan", changed, stubs.secureCallCount)
	}
}

// One failing secondary must not abort the pass: the others still get repaired,
// and the error is surfaced so the caller can warn.
func TestEnforceGroupSecondaries_continuesPastFailure(t *testing.T) {
	groupSetup(t)
	mustAdd(t, mainSite("astrolov", true))
	mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", false))
	mustAdd(t, secondarySite("astrolov-3", "astrolov", "shop", false))

	// Fail the first secondary only; the cert stub has no per-site hook, so
	// clear the error once it has fired.
	origSecure := secureCertFn
	calls := 0
	secureCertFn = func(s config.Site) error {
		calls++
		if calls == 1 {
			return errors.New("issuing certificate: boom")
		}
		return nil
	}
	t.Cleanup(func() { secureCertFn = origSecure })

	changed, err := EnforceGroupSecondaries()
	if err == nil {
		t.Error("expected the failure to be surfaced")
	}
	if len(changed) != 1 {
		t.Fatalf("changed = %v, want exactly one repaired secondary", changed)
	}
	if !reloadSite(t, changed[0]).Secured {
		t.Error("a failure on one secondary must not skip the next")
	}
}

// The install reconcile runs before nginx is started, so a reload against a
// stopped container is expected. Everything else already landed on disk, so the
// secondary counts as repaired rather than failed.
func TestEnforceGroupSecondaries_toleratesStoppedNginx(t *testing.T) {
	stubs := groupSetup(t)
	stubs.reloadErr = nginx.ErrNotRunning
	mustAdd(t, mainSite("astrolov", true))
	mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", false))

	changed, err := EnforceGroupSecondaries()
	if err != nil {
		t.Fatalf("a stopped nginx must not fail the pass: %v", err)
	}
	if len(changed) != 1 || changed[0] != "astrolov-2" {
		t.Fatalf("changed = %v, want [astrolov-2]", changed)
	}
	if !reloadSite(t, "astrolov-2").Secured {
		t.Error("secondary should still be secured on disk")
	}
}

// Securing the main is the most natural way to create the broken combination,
// so the cascade has to run there and not only in the install reconcile.
func TestSetSecured_securingMainCascadesToSecondaries(t *testing.T) {
	groupSetup(t)
	dir := withTempEnv(t)
	main := &config.Site{Name: "astrolov", Domains: []string{"astrolov.test"}, Path: dir, Group: "astrolov"}
	mustAdd(t, *main)
	mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", false))
	mustAdd(t, secondarySite("astrolov-3", "astrolov", "shop", true))
	mustAdd(t, secondarySite("other-2", "other", "blog", false))

	if err := SetSecured(main, true); err != nil {
		t.Fatalf("SetSecured: %v", err)
	}
	if !reloadSite(t, "astrolov-2").Secured {
		t.Error("http secondary was not lifted when its main was secured")
	}
	if !reloadSite(t, "astrolov-3").Secured {
		t.Error("already-secured secondary should stay secured")
	}
	if reloadSite(t, "other-2").Secured {
		t.Error("a secondary of another group must not be touched")
	}
}

// The cascade is reported back so `lerd secure` can name the sites it changed
// rather than securing them behind the user's back.
func TestSetSecuredCascade_reportsTheSecondariesItSecured(t *testing.T) {
	groupSetup(t)
	dir := withTempEnv(t)
	main := &config.Site{Name: "astrolov", Domains: []string{"astrolov.test"}, Path: dir, Group: "astrolov"}
	mustAdd(t, *main)
	mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", false))
	mustAdd(t, secondarySite("astrolov-3", "astrolov", "shop", true))

	cascaded, err := SetSecuredCascade(main, true)
	if err != nil {
		t.Fatalf("SetSecuredCascade: %v", err)
	}
	if len(cascaded) != 1 || cascaded[0] != "astrolov-2" {
		t.Fatalf("cascaded = %v, want only the secondary that changed", cascaded)
	}
}

// Nothing to report for a standalone site, so the caller prints no extra line.
func TestSetSecuredCascade_reportsNothingForAStandaloneSite(t *testing.T) {
	groupSetup(t)
	dir := withTempEnv(t)
	site := &config.Site{Name: "solo", Domains: []string{"solo.test"}, Path: dir}
	mustAdd(t, *site)

	cascaded, err := SetSecuredCascade(site, true)
	if err != nil {
		t.Fatalf("SetSecuredCascade: %v", err)
	}
	if len(cascaded) != 0 {
		t.Errorf("cascaded = %v, want none", cascaded)
	}
}

// Unsecuring a secondary under a secured main would hand its subdomain straight
// back to the main's wildcard, so it is refused with a message rather than
// silently reverted by the next reconcile.
func TestSetSecured_refusesUnsecuringSecondaryUnderSecuredMain(t *testing.T) {
	groupSetup(t)
	dir := withTempEnv(t)
	mustAdd(t, mainSite("astrolov", true))
	sec := &config.Site{
		Name: "astrolov-2", Domains: []string{"blog.astrolov.test"}, Path: dir,
		Secured: true, Group: "astrolov", GroupSubdomain: "blog",
	}
	mustAdd(t, *sec)

	err := SetSecured(sec, false)
	if err == nil {
		t.Fatal("unsecuring a secondary under a secured main should be refused")
	}
	if !strings.Contains(err.Error(), "astrolov") {
		t.Errorf("error should name the main site: %v", err)
	}
	if !reloadSite(t, "astrolov-2").Secured {
		t.Error("the refused toggle must leave the secondary secured")
	}
}

// The main has no 443 wildcard, so the secondary is free to drop to http.
func TestSetSecured_allowsUnsecuringSecondaryUnderUnsecuredMain(t *testing.T) {
	groupSetup(t)
	dir := withTempEnv(t)
	mustAdd(t, mainSite("astrolov", false))
	sec := &config.Site{
		Name: "astrolov-2", Domains: []string{"blog.astrolov.test"}, Path: dir,
		Secured: true, Group: "astrolov", GroupSubdomain: "blog",
	}
	mustAdd(t, *sec)

	if err := SetSecured(sec, false); err != nil {
		t.Fatalf("SetSecured: %v", err)
	}
	if reloadSite(t, "astrolov-2").Secured {
		t.Error("secondary should have dropped to http")
	}
}

// Unsecuring the main is allowed and leaves the secondaries alone: they keep
// their own certs, and no wildcard exists on 443 to swallow them.
func TestSetSecured_unsecuringMainLeavesSecondaries(t *testing.T) {
	groupSetup(t)
	dir := withTempEnv(t)
	main := &config.Site{Name: "astrolov", Domains: []string{"astrolov.test"}, Path: dir, Secured: true, Group: "astrolov"}
	mustAdd(t, *main)
	mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", true))

	if err := SetSecured(main, false); err != nil {
		t.Fatalf("SetSecured: %v", err)
	}
	if !reloadSite(t, "astrolov-2").Secured {
		t.Error("secondary should keep its own https")
	}
}
