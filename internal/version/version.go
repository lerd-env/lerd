// Package version holds build-time version information injected via ldflags.
package version

import (
	"fmt"
	"regexp"
	"strings"
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

// devVersion matches the `git describe` versions a build from a checkout
// carries, which change with every commit.
var devVersion = regexp.MustCompile(`-\d+-g[0-9a-f]+`)

// IsRelease reports whether v is one a user could have installed from a release,
// as opposed to a build from a checkout.
func IsRelease(v string) bool {
	if v == "" || v == "dev" || strings.Contains(v, "dirty") {
		return false
	}
	return !devVersion.MatchString(v)
}
