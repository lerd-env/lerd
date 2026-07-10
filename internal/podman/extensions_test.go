package podman

import "testing"

// TestBundledExtensionsCoverContainerfile guards the invariant that broke in
// #837: every extension the FPM Containerfile installs via docker-php-ext-install
// must be listed in BundledExtensions, so `lerd park` warns accurately about what
// a project's composer.json needs. Reuses the FPM parser from the parity test.
func TestBundledExtensionsCoverContainerfile(t *testing.T) {
	cf, err := GetQuadletTemplate("lerd-php-fpm.Containerfile")
	if err != nil {
		t.Fatalf("reading FPM Containerfile: %v", err)
	}
	installed := fpmDockerExtInstallList(cf)
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
