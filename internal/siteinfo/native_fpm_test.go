package siteinfo

import "testing"

// Under the native runtime a plain FPM site has no container, so looking for one
// reported every serving site as not running (the dashboard read "0/13 running").
// The honest signal is whether the version's host pool is accepting connections.
func TestEnrichFPMUsesTheHostPoolUnderNative(t *testing.T) {
	prevMode, prevPool := nativeRuntimeFn, nativePoolRunningFn
	prevContainer := containerRunningFn
	t.Cleanup(func() {
		nativeRuntimeFn, nativePoolRunningFn, containerRunningFn = prevMode, prevPool, prevContainer
	})

	containerRunningFn = func(string) (bool, error) {
		t.Error("native runtime must not look for an FPM container")
		return false, nil
	}
	nativeRuntimeFn = func() bool { return true }

	nativePoolRunningFn = func(version string) bool {
		if version != "8.5" {
			t.Errorf("asked about version %q, want 8.5", version)
		}
		return true
	}

	e := &EnrichedSite{PHPVersion: "8.5"}
	e.enrichFPM()
	if !e.FPMRunning {
		t.Error("a site whose host pool answers must read as running")
	}

	nativePoolRunningFn = func(string) bool { return false }
	down := &EnrichedSite{PHPVersion: "8.5"}
	down.enrichFPM()
	if down.FPMRunning {
		t.Error("a site whose host pool is down must not read as running")
	}
}

// A custom-container or frankenphp site keeps its container check even under the
// native runtime: the switch only moves plain FPM sites onto the host.
func TestEnrichFPMKeepsContainerSitesUnderNative(t *testing.T) {
	prevMode, prevContainer := nativeRuntimeFn, containerRunningFn
	t.Cleanup(func() { nativeRuntimeFn, containerRunningFn = prevMode, prevContainer })

	nativeRuntimeFn = func() bool { return true }
	asked := ""
	containerRunningFn = func(name string) (bool, error) {
		asked = name
		return true, nil
	}
	e := &EnrichedSite{PHPVersion: "8.5", Runtime: "frankenphp", Name: "shop"}
	e.enrichFPM()
	if asked != "lerd-fp-shop" || !e.FPMRunning {
		t.Errorf("frankenphp site asked %q, running=%v", asked, e.FPMRunning)
	}
}
