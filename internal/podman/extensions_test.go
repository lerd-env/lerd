package podman

import "testing"

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
