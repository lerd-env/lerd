package linker

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The PHP a site needs is built before the site is registered. Registering first
// let the watcher start its worktrees and workers on a version with no image,
// and a failed build left a site lerd went on to set up with nothing to serve it.
func TestApply_buildsTheMissingPHPBeforeRegistering(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // a regression must not reach the real podman
	withFPMImageExists(t, false)

	dir := t.TempDir()
	plan := &Plan{Mode: ModeFPM, Dir: dir, Site: config.Site{
		Name: "shop", Path: dir, Domains: []string{"shop.test"}, PHPVersion: "8.3",
	}}
	registered := func() bool { s, err := config.FindSite("shop"); return err == nil && s != nil }

	built, registeredAtBuild := "", false
	d := Deps{EnsureFPMQuadlet: func(v string) error {
		built, registeredAtBuild = v, registered()
		return errors.New("offline")
	}}
	_, err := Apply(plan, Policy{ImageBuild: true, DeferPublish: true}, d, nil)

	if built != "8.3" || registeredAtBuild {
		t.Errorf("built %q with the site registered=%v, want 8.3 before registering", built, registeredAtBuild)
	}
	if err == nil || registered() {
		t.Errorf("err = %v, registered = %v; a failed build must stop the link", err, registered())
	}
}
