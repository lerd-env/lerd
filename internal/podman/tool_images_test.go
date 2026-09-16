package podman

import (
	"strings"
	"testing"
)

// S3 buckets used to be provisioned by running the minio client image, until
// Docker Hub stopped answering for it on every tag and a fresh install could
// not create a bucket at all. lerd speaks S3 itself now, so no tool image may
// carry a client whose registry lerd does not control.
func TestToolImagesCarryNoS3Client(t *testing.T) {
	for _, img := range ToolImages() {
		if strings.Contains(img, "minio") || strings.Contains(img, "rclone") {
			t.Errorf("tool image %q is an S3 client; S3 is served by the compiled-in client", img)
		}
	}
}
