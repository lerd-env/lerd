package grouping

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Grouping a plain-HTTP site under a secured main must lift it to HTTPS in the
// same step, otherwise it is unreachable over https the moment it joins: the
// main serves "<main> *.<main>", so the wildcard answers the new subdomain.
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

// The inherited flag must reach .lerd.yaml too. Without a committed intent a
// dns disable/enable round trip has nothing to restore from, which is the shape
// the broken site in #811 was in.
func TestAssignSecondary_commitsInheritedSecuredToProjectConfig(t *testing.T) {
	setup(t)
	blogDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(blogDir, ".lerd.yaml"), []byte("domains:\n  - blog.test\n"), 0644); err != nil {
		t.Fatalf("seed .lerd.yaml: %v", err)
	}
	mustAdd(t, config.Site{Name: "astrolov", Domains: []string{"astrolov.test"}, Path: "/srv/astrolov", Secured: true})
	mustAdd(t, config.Site{Name: "blog", Domains: []string{"blog.test"}, Path: blogDir})

	if err := AssignSecondary(reload(t, "astrolov"), reload(t, "blog"), "blog", false); err != nil {
		t.Fatalf("AssignSecondary: %v", err)
	}
	cfg, err := config.LoadProjectConfig(blogDir)
	if err != nil || cfg == nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}
	if !cfg.Secured {
		t.Error(".lerd.yaml should record the inherited HTTPS intent")
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

// A rolled-back grouping must not commit HTTPS intent to .lerd.yaml either.
func TestAssignSecondary_rollbackLeavesProjectConfigOnHTTP(t *testing.T) {
	setup(t)
	regenerateSecondary = func(_ *config.Site, _ string) error { return errors.New("boom") }
	blogDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(blogDir, ".lerd.yaml"), []byte("domains:\n  - blog.test\n"), 0644); err != nil {
		t.Fatalf("seed .lerd.yaml: %v", err)
	}
	mustAdd(t, config.Site{Name: "astrolov", Domains: []string{"astrolov.test"}, Path: "/srv/astrolov", Secured: true})
	mustAdd(t, config.Site{Name: "blog", Domains: []string{"blog.test"}, Path: blogDir})

	if err := AssignSecondary(reload(t, "astrolov"), reload(t, "blog"), "blog", false); err == nil {
		t.Fatal("expected the regen failure to surface")
	}
	if cfg, _ := config.LoadProjectConfig(blogDir); cfg != nil && cfg.Secured {
		t.Error("a failed grouping must not leave secured: true in .lerd.yaml")
	}
}

// The real regeneration issues the cert before writing the vhost that points at
// it, and surfaces a failure instead of logging it: an SSL vhost referencing a
// missing cert file makes the next nginx start fail for every site.
func TestRegenerateSecondary_certFailureAbortsBeforeVhost(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var order []string
	origReissue, origVhost := reissueCertFn, regenerateVhostFn
	reissueCertFn = func(_ config.Site) error {
		order = append(order, "cert")
		return errors.New("mkcert exploded")
	}
	regenerateVhostFn = func(_ *config.Site, _ string) error {
		order = append(order, "vhost")
		return nil
	}
	t.Cleanup(func() { reissueCertFn, regenerateVhostFn = origReissue, origVhost })

	sec := &config.Site{Name: "blog", Domains: []string{"blog.astrolov.test"}, Path: t.TempDir(), Secured: true}
	if err := regenerateSecondary(sec, "blog.test"); err == nil {
		t.Fatal("a cert failure must abort the regeneration, not just log")
	}
	if len(order) != 1 || order[0] != "cert" {
		t.Errorf("call order = %v, want the cert issued first and no vhost written", order)
	}
}
