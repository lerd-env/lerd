package cleanup

import "testing"

// A quadlet lerd writes names its image without a registry
// ("Image=lerd-php83-fpm:local"), while podman tags the built image
// "localhost/lerd-php83-fpm:local". canonRef resolves a registry-less name to
// Docker Hub, so the protected entry used to land under a key no image can
// hold: the quadlet walk protected none of lerd's own images, and a pool that
// was merely stopped lost its image to cleanup and could not start again.
func TestRealProtectedImages_protectsTheLocalhostFormOfAQuadletRef(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	realInstalled, realContainers := installedServiceImages, containerImages
	// The bare form the FPM quadlet carried, and no running container holding
	// it, which is the whole point: a stopped pool must still be protected.
	installedServiceImages = func() []string { return []string{"lerd-php83-fpm:local"} }
	containerImages = func() []string { return nil }
	t.Cleanup(func() { installedServiceImages, containerImages = realInstalled, realContainers })

	prot, err := realProtectedImages()
	if err != nil {
		t.Fatalf("realProtectedImages: %v", err)
	}
	if !prot["localhost/lerd-php83-fpm:local"] {
		t.Errorf("localhost form not protected; protected set = %v", prot)
	}
}

func TestLocalhostRef(t *testing.T) {
	cases := map[string]string{
		"lerd-php83-fpm:local":           "localhost/lerd-php83-fpm:local",
		"lerd-custom-myapp:local":        "localhost/lerd-custom-myapp:local",
		"localhost/lerd-php83-fpm:local": "", // already carries a registry
		"docker.io/library/mysql:8.4":    "", // ditto
		"ghcr.io/lerd-env/base:abc":      "",
	}
	for in, want := range cases {
		if got := localhostRef(in); got != want {
			t.Errorf("localhostRef(%q) = %q, want %q", in, got, want)
		}
	}
}
