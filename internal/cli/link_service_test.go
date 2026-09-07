package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

type siteStateCall struct {
	Service string
	Site    string
}

// stubLinkServiceSeams replaces the two seams that need a live podman: starting
// the service and provisioning the site's state on it.
func stubLinkServiceSeams(t *testing.T, startErr error) *[]siteStateCall {
	t.Helper()
	var calls []siteStateCall
	prevRun := ensureServiceRunning
	prevState := linkEnsureSiteState
	ensureServiceRunning = func(string) error { return startErr }
	linkEnsureSiteState = func(service string, s config.Site) (string, error) {
		calls = append(calls, siteStateCall{Service: service, Site: s.Name})
		return "created bucket " + s.Name, nil
	}
	t.Cleanup(func() {
		ensureServiceRunning = prevRun
		linkEnsureSiteState = prevState
	})
	return &calls
}

// SQLite is a per-project file, not a container. linkApplyServices must skip it
// rather than route it through ensureServiceRunning, which looks for a preset or
// custom service YAML and warns "custom service sqlite not found" when neither
// exists (there is no sqlite preset by design).
func TestLinkApplyServices_SkipsSQLite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()

	proj := &config.ProjectConfig{Services: []config.ProjectService{{Name: "sqlite"}}}
	if err := linkApplyServices(t.TempDir(), config.Site{Name: "demo"}, proj); err != nil {
		t.Fatalf("linkApplyServices: %v", err)
	}
	if strings.Contains(buf.String(), "sqlite") {
		t.Errorf("sqlite should be skipped silently, got output: %q", buf.String())
	}
}

// Linking a site to an object-storage service is the moment its bucket has to
// appear; without this the site's .env points at a bucket nothing ever creates.
func TestLinkApplyServices_EnsuresSiteStateForEachService(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", dir)

	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()
	calls := stubLinkServiceSeams(t, nil)

	site := config.Site{Name: "uploads", Path: dir}
	proj := &config.ProjectConfig{Services: []config.ProjectService{{Name: "rustfs"}}}
	if err := linkApplyServices(dir, site, proj); err != nil {
		t.Fatalf("linkApplyServices: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0] != (siteStateCall{Service: "rustfs", Site: "uploads"}) {
		t.Errorf("expected one site-state call for rustfs/uploads, got %v", *calls)
	}
}

func TestLinkApplyServices_ServiceThatWontStart_SkipsSiteState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", dir)

	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()
	calls := stubLinkServiceSeams(t, errors.New("unit failed"))

	proj := &config.ProjectConfig{Services: []config.ProjectService{{Name: "rustfs"}}}
	if err := linkApplyServices(dir, config.Site{Name: "uploads", Path: dir}, proj); err != nil {
		t.Fatalf("linkApplyServices: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("site state must not be provisioned against a service that did not start, got %v", *calls)
	}
}
