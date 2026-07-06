// Package origin centralises every URL lerd fetches its own assets from: release
// binaries, the framework and service stores, the changelog, and the GHCR base
// images. Everything is served from the lerd-env org (the geodro->lerd-env move
// is complete), and each endpoint is overridable via its environment variable
// for tests and mirrors.
package origin

import (
	"os"
	"strings"
)

const (
	owner          = "lerd-env"      // GitHub org, also the GHCR namespace
	mainRepo       = "lerd-env/lerd" // releases, installer, changelog
	frameworksRepo = "lerd-env/frameworks"
	servicesRepo   = "lerd-env/services"
)

// StoreBaseURLs returns the framework-store base. The definitions live under a
// frameworks/ subdir (index.json + <name>.yaml), not at the repo root.
func StoreBaseURLs() []string {
	if list := splitList(os.Getenv("LERD_STORE_BASE_URL")); len(list) > 0 {
		return list
	}
	return []string{"https://raw.githubusercontent.com/" + frameworksRepo + "/main/frameworks"}
}

// ServiceStoreBaseURLs returns the service-preset-store base, nested under a
// services/ subdir.
func ServiceStoreBaseURLs() []string {
	if list := splitList(os.Getenv("LERD_SERVICES_BASE_URL")); len(list) > 0 {
		return list
	}
	return []string{"https://raw.githubusercontent.com/" + servicesRepo + "/main/services"}
}

// ReleaseBaseURLs lists GitHub releases bases.
func ReleaseBaseURLs() []string {
	if list := splitList(os.Getenv("LERD_RELEASES_URL")); len(list) > 0 {
		return list
	}
	return []string{"https://github.com/" + mainRepo + "/releases"}
}

// ReleaseDownloadBases lists release-asset download bases.
func ReleaseDownloadBases() []string {
	if list := splitList(os.Getenv("LERD_RELEASE_DOWNLOAD_URL")); len(list) > 0 {
		return list
	}
	out := ReleaseBaseURLs()
	for i := range out {
		out[i] += "/download"
	}
	return out
}

// ReleaseAPIBaseURLs lists GitHub API bases.
func ReleaseAPIBaseURLs() []string {
	if list := splitList(os.Getenv("LERD_RELEASES_API_URL")); len(list) > 0 {
		return list
	}
	return []string{"https://api.github.com/repos/" + mainRepo}
}

// ChangelogURLs lists raw changelog URLs.
func ChangelogURLs() []string {
	if list := splitList(os.Getenv("LERD_CHANGELOG_URL")); len(list) > 0 {
		return list
	}
	return []string{"https://raw.githubusercontent.com/" + mainRepo + "/main/CHANGELOG.md"}
}

// BaseImageRefs lists GHCR refs for a prebuilt PHP-FPM base image, where phpShort
// is the dotless version (e.g. "85") and hash pins the image to the embedded
// Containerfile template.
func BaseImageRefs(phpShort, hash string) []string {
	suffix := "/lerd-php" + phpShort + "-fpm-base:" + hash
	if v := os.Getenv("LERD_BASE_IMAGE_REGISTRY"); v != "" {
		return []string{v + suffix}
	}
	return []string{"ghcr.io/" + owner + suffix}
}

// splitList parses a comma-separated override into trimmed, non-empty entries.
func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
