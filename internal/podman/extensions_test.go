package podman

import "testing"

// TestComposerPlatformNameRoundTrips checks both directions of the name folding:
// composer publishes OPcache as ext-zend-opcache (the module is "Zend OPcache",
// so extension_loaded("opcache") is false), while the image installs it as
// opcache. Both spellings must agree with BundledExtensions.
func TestComposerPlatformNameRoundTrips(t *testing.T) {
	cases := []struct{ bundled, platform string }{
		{"opcache", "zend-opcache"},
		{"redis", "redis"},
		{"pdo_mysql", "pdo_mysql"},
	}
	for _, c := range cases {
		if got := ComposerPlatformName(c.bundled); got != c.platform {
			t.Errorf("ComposerPlatformName(%q) = %q, want %q", c.bundled, got, c.platform)
		}
		if got := CanonicalExtension(c.platform); got != c.bundled {
			t.Errorf("CanonicalExtension(%q) = %q, want %q", c.platform, got, c.bundled)
		}
	}
}

func TestCanonicalExtensionIsCaseInsensitive(t *testing.T) {
	if got := CanonicalExtension("Zend-OPcache"); got != "opcache" {
		t.Errorf("CanonicalExtension(%q) = %q, want %q", "Zend-OPcache", got, "opcache")
	}
}

// TestBundledExtensionsUseInstallNames guards the mapping's premise: every alias
// key is a name BundledExtensions actually lists, so a rename in the list cannot
// silently orphan its composer counterpart.
func TestBundledExtensionsUseInstallNames(t *testing.T) {
	bundled := map[string]bool{}
	for _, e := range BundledExtensions() {
		bundled[e] = true
	}
	for install := range composerPlatformNames {
		if !bundled[install] {
			t.Errorf("composerPlatformNames maps %q, but BundledExtensions does not list it", install)
		}
	}
}

// TestBundledExtensionsCoverContainerfile checks one direction: every extension
// the FPM Containerfile installs or enables (docker-php-ext-install + the pecl
// docker-php-ext-enable names) is listed in BundledExtensions, so adding one to
// the image and forgetting the list is caught. This is NOT the #837 direction
// (BundledExtensions claiming a name nothing installs); that unverifiable core
// group is guarded by the php -m check in base-images.yml (#856).
func TestBundledExtensionsCoverContainerfile(t *testing.T) {
	cf, err := GetQuadletTemplate("lerd-php-fpm.Containerfile")
	if err != nil {
		t.Fatalf("reading FPM Containerfile: %v", err)
	}
	installed := fpmContainerfileExtensions(cf)
	if len(installed) < 10 {
		t.Fatalf("parsed only %d extensions, parser likely broke: %v", len(installed), installed)
	}
	bundled := map[string]bool{}
	for _, e := range BundledExtensions() {
		bundled[e] = true
	}
	for _, ext := range installed {
		if !bundled[ext] {
			t.Errorf("Containerfile installs %q but BundledExtensions omits it — park would never warn a project needs it", ext)
		}
	}
}
