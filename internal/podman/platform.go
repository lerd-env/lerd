package podman

import "strings"

// arm64PostgisImage is the multi-arch (amd64 + arm64) drop-in for the upstream
// postgis/postgis image, which ships no arm64 manifest. Same tags, same
// PostgreSQL + PostGIS versions, same on-disk data dir, so swapping to it is
// transparent. The official postgis README points Apple Silicon users here.
const arm64PostgisImage = "imresamu/postgis"

// rewriteArm64Image swaps an image that ships no arm64 manifest for a multi-arch
// drop-in when goarch is arm64, so Apple Silicon runs it natively instead of
// under Rosetta. Only postgis/postgis -> imresamu/postgis today; the registry
// prefix and the tag are preserved, and the data dir is unchanged (same
// PostgreSQL + PostGIS version). Pure (arch is passed in) so the decision is
// unit-tested off-darwin. Everything else, including mysql:5.7 (legacy, EOL, no
// arm64 fork adopted), is returned unchanged and keeps its amd64 pin.
func rewriteArm64Image(image, goarch string) string {
	if goarch != "arm64" {
		return image
	}
	const upstream = "postgis/postgis"
	if i := strings.Index(image, upstream); i >= 0 {
		return image[:i] + arm64PostgisImage + image[i+len(upstream):]
	}
	return image
}
