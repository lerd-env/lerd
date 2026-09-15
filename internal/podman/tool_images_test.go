package podman

import (
	"strings"
	"testing"
)

// Docker Hub stopped answering for minio/mc on every tag, so a fresh install
// could not create an S3 bucket at all while a cached image hid it on machines
// that already had one. Keep every throw-away tool image off that registry.
func TestToolImagesAvoidDockerHubMinio(t *testing.T) {
	for _, img := range ToolImages() {
		if strings.Contains(img, "docker.io/minio/") {
			t.Errorf("tool image %q is back on Docker Hub, which no longer serves minio/mc", img)
		}
	}
}

func TestMinioClientImageIsPullable(t *testing.T) {
	if !strings.HasPrefix(MinioClientImage, "quay.io/minio/mc") {
		t.Errorf("MinioClientImage = %q, want the quay.io mirror", MinioClientImage)
	}
}
