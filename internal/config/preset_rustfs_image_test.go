package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The buckets entity shells out to mc, so it names the image itself rather than
// going through podman.MinioClientImage. It has to move off Docker Hub with the
// constant, or the Buckets panel breaks on a machine with no cached image.
func TestRustfsBucketsEntityAvoidsDockerHubMinio(t *testing.T) {
	data, err := presetFS.ReadFile("presets/rustfs.yaml")
	if err != nil {
		t.Fatalf("reading the bundled rustfs preset: %v", err)
	}
	var svc CustomService
	if err := yaml.Unmarshal(data, &svc); err != nil {
		t.Fatalf("parsing the bundled rustfs preset: %v", err)
	}
	spec := svc.Introspect.Entity("buckets")
	if spec == nil {
		t.Fatal("the rustfs preset no longer declares a buckets entity")
	}
	if strings.Contains(spec.Image, "docker.io/minio/") {
		t.Errorf("buckets entity image = %q, which Docker Hub no longer serves", spec.Image)
	}
}
