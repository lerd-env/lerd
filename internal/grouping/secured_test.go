package grouping

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// securedSetup extends setup with a stubbed SetSecured so the invariant pass
// can be asserted on registry state without issuing certs or touching nginx.
func securedSetup(t *testing.T) *[]string {
	t.Helper()
	setup(t)
	var calls []string
	origSet, origDNS := setSecuredFn, dnsManagedFn
	setSecuredFn = func(s *config.Site, secured bool) error {
		calls = append(calls, s.Name)
		s.Secured = secured
		return config.AddSite(*s)
	}
	dnsManagedFn = func() bool { return true }
	t.Cleanup(func() { setSecuredFn, dnsManagedFn = origSet, origDNS })
	return &calls
}

func mainSite(name string, secured bool) config.Site {
	return config.Site{Name: name, Domains: []string{name + ".test"}, Path: "/tmp/" + name, Secured: secured, Group: name}
}

func secondarySite(name, group, label string, secured bool) config.Site {
	return config.Site{
		Name: name, Domains: []string{label + "." + group + ".test"}, Path: "/tmp/" + name,
		Secured: secured, Group: group, GroupSubdomain: label,
	}
}

// The bug: a secondary left on plain HTTP under a secured parent has no 443
// block, so the parent's *.parent wildcard answers its subdomain over HTTPS
// and serves the wrong app (issue #811).
func TestEnforceSecondarySecured_securesHTTPSecondaryUnderSecuredParent(t *testing.T) {
	calls := securedSetup(t)
	mustAdd(t, mainSite("astrolov", true))
	mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", false))

	changed, err := EnforceSecondarySecured()
	if err != nil {
		t.Fatalf("EnforceSecondarySecured: %v", err)
	}
	if len(changed) != 1 || changed[0] != "astrolov-2" {
		t.Fatalf("changed = %v, want [astrolov-2]", changed)
	}
	if got := reload(t, "astrolov-2"); !got.Secured {
		t.Error("secondary should be secured to match its parent")
	}
	if len(*calls) != 1 {
		t.Errorf("SetSecured calls = %v, want exactly one", *calls)
	}
}

func TestEnforceSecondarySecured_leavesConsistentGroupsAlone(t *testing.T) {
	cases := []struct {
		name             string
		main, secondary  bool
		wantSecuredAfter bool
	}{
		// Already consistent: nothing to do, and no cert churn.
		{"both secured", true, true, true},
		// An unsecured parent has no 443 block, so nothing swallows the
		// secondary. A secondary may legitimately be HTTPS on its own.
		{"unsecured parent, secured secondary", false, true, true},
		{"both plain http", false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := securedSetup(t)
			mustAdd(t, mainSite("astrolov", c.main))
			mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", c.secondary))

			changed, err := EnforceSecondarySecured()
			if err != nil {
				t.Fatalf("EnforceSecondarySecured: %v", err)
			}
			if len(changed) != 0 {
				t.Errorf("changed = %v, want none", changed)
			}
			if len(*calls) != 0 {
				t.Errorf("SetSecured called %v, want no cert churn", *calls)
			}
			if got := reload(t, "astrolov-2"); got.Secured != c.wantSecuredAfter {
				t.Errorf("secondary secured = %v, want %v", got.Secured, c.wantSecuredAfter)
			}
		})
	}
}

// With DNS off there is no HTTPS at all, so every site is plain HTTP and the
// wildcard cannot swallow anything. Enforcing would only fail on ErrDNSDisabled.
func TestEnforceSecondarySecured_noopWhenDNSDisabled(t *testing.T) {
	calls := securedSetup(t)
	dnsManagedFn = func() bool { return false }
	mustAdd(t, mainSite("astrolov", true))
	mustAdd(t, secondarySite("astrolov-2", "astrolov", "blog", false))

	changed, err := EnforceSecondarySecured()
	if err != nil {
		t.Fatalf("EnforceSecondarySecured: %v", err)
	}
	if len(changed) != 0 || len(*calls) != 0 {
		t.Errorf("changed=%v calls=%v, want no action with DNS disabled", changed, *calls)
	}
}

// A standalone site that happens to be unsecured must never be touched: only
// secondaries inherit, and only from their own group's main.
func TestEnforceSecondarySecured_ignoresUngroupedAndOtherGroups(t *testing.T) {
	calls := securedSetup(t)
	mustAdd(t, config.Site{Name: "solo", Domains: []string{"solo.test"}, Path: "/tmp/solo"})
	mustAdd(t, mainSite("astrolov", true))
	mustAdd(t, mainSite("other", false))
	mustAdd(t, secondarySite("other-2", "other", "blog", false))

	changed, err := EnforceSecondarySecured()
	if err != nil {
		t.Fatalf("EnforceSecondarySecured: %v", err)
	}
	if len(changed) != 0 || len(*calls) != 0 {
		t.Errorf("changed=%v calls=%v, want no action", changed, *calls)
	}
}

// A secondary whose group has no main in the registry has no wildcard above it.
func TestEnforceSecondarySecured_orphanSecondaryIsLeftAlone(t *testing.T) {
	calls := securedSetup(t)
	mustAdd(t, secondarySite("orphan", "gone", "blog", false))

	changed, err := EnforceSecondarySecured()
	if err != nil {
		t.Fatalf("EnforceSecondarySecured: %v", err)
	}
	if len(changed) != 0 || len(*calls) != 0 {
		t.Errorf("changed=%v calls=%v, want no action for an orphan", changed, *calls)
	}
}

// Grouping a plain-HTTP site under a secured main must lift it to HTTPS in the
// same step, otherwise it is unreachable over https the moment it joins.
func TestAssignSecondary_inheritsSecuredFromMain(t *testing.T) {
	setup(t)
	mustAdd(t, config.Site{Name: "astrolov", Domains: []string{"astrolov.test"}, Path: "/srv/astrolov", Secured: true})
	mustAdd(t, config.Site{Name: "blog", Domains: []string{"blog.test"}, Path: "/srv/blog"})

	if err := AssignSecondary(reload(t, "astrolov"), reload(t, "blog"), "blog", false); err != nil {
		t.Fatalf("AssignSecondary: %v", err)
	}
	if got := reload(t, "blog"); !got.Secured {
		t.Error("secondary joining a secured group should inherit HTTPS")
	}
}

// An unsecured main has no 443 wildcard, so joining it must not silently drop a
// secondary that was already serving HTTPS on its own.
func TestAssignSecondary_keepsSecuredUnderUnsecuredMain(t *testing.T) {
	setup(t)
	mustAdd(t, config.Site{Name: "astrolov", Domains: []string{"astrolov.test"}, Path: "/srv/astrolov"})
	mustAdd(t, config.Site{Name: "blog", Domains: []string{"blog.test"}, Path: "/srv/blog", Secured: true})

	if err := AssignSecondary(reload(t, "astrolov"), reload(t, "blog"), "blog", false); err != nil {
		t.Fatalf("AssignSecondary: %v", err)
	}
	if got := reload(t, "blog"); !got.Secured {
		t.Error("secondary must keep its own HTTPS under an unsecured main")
	}
}

// A rolled-back grouping must not leave the inherited HTTPS flag behind.
func TestAssignSecondary_rollsBackInheritedSecured(t *testing.T) {
	setup(t)
	regenerateSecondary = func(_ *config.Site, _ string) error { return errors.New("boom") }
	mustAdd(t, config.Site{Name: "astrolov", Domains: []string{"astrolov.test"}, Path: "/srv/astrolov", Secured: true})
	mustAdd(t, config.Site{Name: "blog", Domains: []string{"blog.test"}, Path: "/srv/blog"})

	if err := AssignSecondary(reload(t, "astrolov"), reload(t, "blog"), "blog", false); err == nil {
		t.Fatal("expected the regen failure to surface")
	}
	if got := reload(t, "blog"); got.Secured {
		t.Error("rollback should restore the secondary's original HTTP state")
	}
}

// One failing secondary must not abort the pass: the others still get repaired,
// and the error is surfaced so the caller can warn.
func TestEnforceSecondarySecured_continuesPastFailure(t *testing.T) {
	securedSetup(t)
	setSecuredFn = func(s *config.Site, secured bool) error {
		if s.Name == "bad" {
			return errors.New("issuing certificate: boom")
		}
		s.Secured = secured
		return config.AddSite(*s)
	}
	mustAdd(t, mainSite("astrolov", true))
	mustAdd(t, secondarySite("bad", "astrolov", "bad", false))
	mustAdd(t, secondarySite("good", "astrolov", "good", false))

	changed, err := EnforceSecondarySecured()
	if err == nil {
		t.Error("expected the failure to be surfaced")
	}
	if len(changed) != 1 || changed[0] != "good" {
		t.Fatalf("changed = %v, want [good]", changed)
	}
	if got := reload(t, "good"); !got.Secured {
		t.Error("a failure on one secondary must not skip the next")
	}
}
