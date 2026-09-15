package podman

// lerd runs a few public images as throw-away containers rather than as
// services: no quadlet names them and no container outlives the command, so
// nothing in lerd's config references them. They are listed here so cleanup can
// tell them apart from an image nobody wants, and so the refs live in one place
// instead of being repeated at each call site.
const (
	// ProbeImage backs the IPv6 network probe, which runs it with --pull never:
	// removing it downgrades the probe to inconclusive rather than pulling.
	ProbeImage = "alpine:latest"
	// MinioClientImage provisions S3 buckets on the rustfs/minio services.
	// Pulled from quay.io rather than Docker Hub: docker.io/minio/mc stopped
	// answering for every tag, so a fresh install could not create a bucket at
	// all while a cached image hid it on machines that already had one.
	MinioClientImage = "quay.io/minio/mc:latest"
)

// ToolImages lists every image lerd runs as a throw-away container.
func ToolImages() []string {
	return []string{ProbeImage, MinioClientImage}
}
