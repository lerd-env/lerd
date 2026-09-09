package cli

import "testing"

// Pinning a site to a version with no image used to repoint the vhost at a
// backend that could not start: the FPM unit failed on
// `short-name "lerd-php83-fpm:local" did not resolve` and the site served 502
// until the image was built by hand. The image is built before the switch now.
func TestIsolateBuildsAMissingImageFirst(t *testing.T) {
	origExists, origProvision := isolateImageExistsFn, isolateProvisionFn
	t.Cleanup(func() { isolateImageExistsFn, isolateProvisionFn = origExists, origProvision })

	built := ""
	isolateImageExistsFn = func(string) bool { return false }
	isolateProvisionFn = func(v string) { built = v }

	ensureIsolateImage("8.3")

	if built != "8.3" {
		t.Fatalf("missing image was not built, got %q", built)
	}
}

// A version already built is switched to without touching the network.
func TestIsolateSkipsBuildWhenImageExists(t *testing.T) {
	origExists, origProvision := isolateImageExistsFn, isolateProvisionFn
	t.Cleanup(func() { isolateImageExistsFn, isolateProvisionFn = origExists, origProvision })

	called := false
	isolateImageExistsFn = func(string) bool { return true }
	isolateProvisionFn = func(string) { called = true }

	ensureIsolateImage("8.5")

	if called {
		t.Fatal("an existing image should not be rebuilt")
	}
}
