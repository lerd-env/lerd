package serviceops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// A removed service stays removed: nothing a site does may start it again,
// until the user installs it on purpose.
func TestRemoveService_isRememberedUntilInstalledAgain(t *testing.T) {
	withServiceHome(t)
	stubPodmanRemove(t)
	stubLifecycle(t)
	if err := config.SaveCustomService(&config.CustomService{Name: "phpmyadmin", Image: "docker.io/library/alpine:latest"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveService("phpmyadmin", RemoveOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if !config.ServiceIsRemoved("phpmyadmin") {
		t.Fatal("removal was not remembered")
	}
	err := EnsureServiceRunning("phpmyadmin")
	if err == nil || !strings.Contains(err.Error(), "was removed") {
		t.Fatalf("EnsureServiceRunning on a removed service = %v, want refused", err)
	}

	if err := registerPreset(&config.CustomService{Name: "phpmyadmin", Image: "docker.io/library/alpine:latest"}); err != nil {
		t.Fatal(err)
	}
	if config.ServiceIsRemoved("phpmyadmin") {
		t.Fatal("installing it again kept the removal")
	}
}

func TestServiceOrphaned(t *testing.T) {
	withServiceHome(t)
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, "lerd-"+name+".container"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("phpmyadmin", podman.CustomServiceQuadletMarker+"\n[Container]\n")
	write("adminer", podman.CustomServiceQuadletMarker+"\n[Container]\n")
	write("vite-shop", "[Container]\n") // a worker unit, not a service
	if err := config.SaveCustomService(&config.CustomService{Name: "adminer", Image: "docker.io/library/alpine:latest"}); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]bool{"phpmyadmin": true, "adminer": false, "vite-shop": false, "nothing": false} {
		if got := ServiceOrphaned(name); got != want {
			t.Errorf("ServiceOrphaned(%q) = %v, want %v", name, got, want)
		}
	}
}
