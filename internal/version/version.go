// Package version holds build-time version information injected via ldflags.
package version

import (
	"fmt"
	"regexp"
)

// These variables are set at build time via:
//
//	-X github.com/geodro/lerd/internal/version.Version=<tag>
//	-X github.com/geodro/lerd/internal/version.Commit=<sha>
//	-X github.com/geodro/lerd/internal/version.Date=<iso8601>
var (
	Version = "1.37.0"
	Commit  = "none"
	Date    = "unknown"
)

// String returns the full version string shown by `lerd --version`.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}

// releaseVersion is the shape of a tagged release, 1.38.0 or 1.38.0-beta.1.
// A build from a checkout carries `git describe` output instead, or a bare
// commit hash when no tag is reachable, and neither matches.
var releaseVersion = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

// IsRelease reports whether v is one a user could have installed from a release,
// as opposed to a build from a checkout.
func IsRelease(v string) bool { return releaseVersion.MatchString(v) }
