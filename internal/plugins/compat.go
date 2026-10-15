package plugins

import (
	"regexp"
	"strconv"

	"github.com/geodro/lerd/internal/version"
)

// runningVersion is the lerd a plugin is judged against. A var so tests can
// pretend to be another release.
var runningVersion = version.Version

var (
	releaseRe    = regexp.MustCompile(`^v?(\d+)\.(\d+)`)
	lowerBoundRe = regexp.MustCompile(`^>=\s*(\d+)\.(\d+)(\.\d+)?$`)
)

// Compatible reports whether this lerd meets a plugin's `lerd` constraint,
// compared on major and minor. A plugin states the oldest lerd it needs and no
// newest, so a new release never strands it; breaking the contract is what
// api_version is for, and any other form of constraint is refused. A build
// from an untagged commit has no version to judge and loads everything.
// Compared here rather than with internal/php, which would link the service
// managers into every package that emits a plugin event.
func Compatible(constraint string) bool {
	if constraint == "" {
		return true
	}
	need := lowerBoundRe.FindStringSubmatch(constraint)
	if need == nil {
		return false
	}
	have := releaseRe.FindStringSubmatch(runningVersion)
	if have == nil {
		return true
	}
	if a, b := atoi(have[1]), atoi(need[1]); a != b {
		return a > b
	}
	return atoi(have[2]) >= atoi(need[2])
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
