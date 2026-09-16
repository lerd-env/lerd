package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// Listing and changing buckets is served by the compiled-in S3 client: the
// client images that used to do it are public images lerd does not control, and
// when Docker Hub stopped serving minio/mc the Buckets panel broke on every
// machine with no cached image.
func TestRustfsBucketsEntityIsDriven(t *testing.T) {
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
	if spec.Driver != "s3" || spec.Image != "" {
		t.Errorf("buckets entity driver = %q, image = %q, want the s3 driver and no client image", spec.Driver, spec.Image)
	}
	for _, key := range []string{"S3_PORT", "S3_ACCESS_KEY", "S3_SECRET_KEY"} {
		found := false
		for _, kv := range spec.Env {
			if len(kv) > len(key) && kv[:len(key)] == key {
				found = true
			}
		}
		if !found {
			t.Errorf("buckets entity env is missing %s", key)
		}
	}
}
