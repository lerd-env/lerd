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

// stripe-cli 1.51 made listen refuse to start without an event selection, and
// :latest moved onto it under every install; a listener has to name its events
// on an image that knows the flag.
func TestStripeListenExecStart_NamesItsEventsOnAPinnedImage(t *testing.T) {
	got := StripeListenExecStart("lerd-stripe-shop", "sk_test_x", "https://shop.test/stripe/webhook")
	for _, want := range []string{
		" run --rm --replace --name lerd-stripe-shop --network host ",
		StripeCLIImage + " listen --api-key sk_test_x --forward-to https://shop.test/stripe/webhook --skip-verify --all-snapshot",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ExecStart = %q, want it to contain %q", got, want)
		}
	}
	if strings.HasSuffix(StripeCLIImage, ":latest") {
		t.Errorf("StripeCLIImage = %q: an older image cached as :latest rejects --all-snapshot", StripeCLIImage)
	}
}
