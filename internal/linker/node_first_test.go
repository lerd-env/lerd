package linker

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The Node a site needs is there before the watcher can start its workers.
func TestApply_installsTheMissingNodeBeforeRegistering(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // a regression must not reach the real podman
	dir := t.TempDir()
	plan := &Plan{Mode: ModeCustomFPM, Dir: dir, Site: config.Site{Name: "shop", Path: dir, Domains: []string{"shop.test"}}}
	registered := func() bool { s, err := config.FindSite("shop"); return err == nil && s != nil }

	called, before := false, false
	boom := errors.New("offline")
	d := Deps{EnsureNode: func(s config.Site, _ Reporter) error {
		called, before = s.Name == "shop", registered()
		return boom
	}}
	_, err := Apply(plan, Policy{}, d, nil)
	if !called || before || !errors.Is(err, boom) || registered() {
		t.Errorf("called=%v registeredBefore=%v err=%v registered=%v", called, before, err, registered())
	}
}

// A site host nginx cannot reach is refused before Node or an image is fetched for it.
func TestApply_refusesAContainerSiteBeforeInstallingNode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if err := config.SetNginxOnHost(true); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	plan := &Plan{Mode: ModeCustomFPM, Dir: dir, Site: config.Site{Name: "api", Path: dir, Domains: []string{"api.test"}, ContainerPort: 3000}}
	called := false
	d := Deps{EnsureNode: func(config.Site, Reporter) error { called = true; return nil }}
	if _, err := Apply(plan, Policy{}, d, nil); err == nil {
		t.Fatal("Apply linked a container site while nginx runs on the host")
	}
	if called {
		t.Error("Node was installed for a site the link then refused")
	}
}
