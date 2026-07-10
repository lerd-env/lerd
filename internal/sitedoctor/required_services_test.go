package sitedoctor

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func withStubs(t *testing.T, installed map[string]bool, status map[string]string) {
	t.Helper()
	origInstalled, origStatus := quadletInstalledFn, unitStatusFn
	quadletInstalledFn = func(unit string) bool { return installed[unit] }
	unitStatusFn = func(unit string) (string, error) { return status[unit], nil }
	t.Cleanup(func() { quadletInstalledFn, unitStatusFn = origInstalled, origStatus })
}

func TestRequiredServicesSkippedWhenNoneDeclared(t *testing.T) {
	withStubs(t, nil, nil)
	if _, ok := checkRequiredServices(&config.Framework{}); ok {
		t.Error("a framework requiring nothing should produce no check")
	}
	if _, ok := checkRequiredServices(nil); ok {
		t.Error("nil framework should produce no check")
	}
}

func TestRequiredServicesOKWhenInstalledAndActive(t *testing.T) {
	withStubs(t, map[string]bool{"lerd-opensearch": true}, map[string]string{"lerd-opensearch": "active"})
	c, ok := checkRequiredServices(&config.Framework{Requires: []string{"opensearch"}})
	if !ok || c.Status != StatusOK {
		t.Fatalf("got %+v", c)
	}
}

// Not installed is a hard failure: the app cannot boot without it.
func TestRequiredServicesFailsWhenNotInstalled(t *testing.T) {
	withStubs(t, nil, nil)
	c, ok := checkRequiredServices(&config.Framework{Label: "Magento", Requires: []string{"opensearch"}})
	if !ok || c.Status != StatusFail {
		t.Fatalf("got %+v", c)
	}
	if !strings.Contains(c.Detail, "opensearch") || !strings.Contains(c.Detail, "lerd service preset") {
		t.Errorf("detail should name the service and the remedy: %q", c.Detail)
	}
}

// Installed but stopped is a warning, not a failure: starting it is one command
// and lerd starts it on the next link anyway.
func TestRequiredServicesWarnsWhenStopped(t *testing.T) {
	withStubs(t, map[string]bool{"lerd-opensearch": true}, map[string]string{"lerd-opensearch": "inactive"})
	c, ok := checkRequiredServices(&config.Framework{Requires: []string{"opensearch"}})
	if !ok || c.Status != StatusWarn {
		t.Fatalf("got %+v", c)
	}
	if !strings.Contains(c.Detail, "lerd service start") {
		t.Errorf("detail should name the remedy: %q", c.Detail)
	}
}

// A missing service outranks a stopped one, since it is the more severe finding.
func TestRequiredServicesMissingOutranksStopped(t *testing.T) {
	withStubs(t,
		map[string]bool{"lerd-redis": true},
		map[string]string{"lerd-redis": "inactive"},
	)
	c, _ := checkRequiredServices(&config.Framework{Requires: []string{"redis", "opensearch"}})
	if c.Status != StatusFail {
		t.Fatalf("got %s, want fail: %+v", c.Status, c)
	}
	if !strings.Contains(c.Detail, "opensearch") {
		t.Errorf("detail should name the missing service: %q", c.Detail)
	}
}
