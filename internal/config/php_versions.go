package config

import (
	"fmt"
	"strconv"
	"strings"
)

// SupportedPHPVersions lists the PHP versions lerd can build FPM images for.
// 7.4 and 8.0 are a frozen legacy tier for old projects: still buildable from
// Alpine 3.16, but pinned (older xdebug, no mongodb ext) and not security-updated.
// The tail may hold a prerelease tier, see PrereleasePHPVersions.
var SupportedPHPVersions = []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5", "8.6"}

// PrereleasePHPVersions is the tier upstream has not released yet: built only on
// request, labelled as a prerelease, FPM only. On release day delete the version
// here; FrankenPHP has its own gate below.
var PrereleasePHPVersions = []string{}

// FrankenPHPMinVersion is the oldest PHP version dunglas/frankenphp publishes an
// image for; supported versions below it run under FPM only.
const FrankenPHPMinVersion = "8.2"

// FrankenPHPMaxVersion is the newest PHP version dunglas/frankenphp publishes an
// image for. It trails PHP releases, so raise it once the image exists.
const FrankenPHPMaxVersion = "8.5"

// IsPrereleasePHPVersion reports whether v is an unreleased upstream version.
func IsPrereleasePHPVersion(v string) bool {
	for _, s := range PrereleasePHPVersions {
		if s == v {
			return true
		}
	}
	return false
}

// StablePHPVersions returns the supported versions upstream has released, in the
// same ascending order. This is what bulk operations act on, so a prerelease is
// never built for someone who did not ask for it by name.
func StablePHPVersions() []string {
	out := make([]string, 0, len(SupportedPHPVersions))
	for _, v := range SupportedPHPVersions {
		if !IsPrereleasePHPVersion(v) {
			out = append(out, v)
		}
	}
	return out
}

// UpstreamPHPTag returns the docker.io/library/php tag an image builds FROM.
// A prerelease has no plain "<v>-fpm-alpine" tag until GA, so it resolves to the
// "-rc" tag upstream publishes throughout beta and RC.
func UpstreamPHPTag(v string) string {
	if IsPrereleasePHPVersion(v) {
		return v + "-rc"
	}
	return v
}

// IsSupportedPHPVersion reports whether v is a version lerd can install.
func IsSupportedPHPVersion(v string) bool {
	for _, s := range SupportedPHPVersions {
		if s == v {
			return true
		}
	}
	return false
}

// NormalizePHPVersion reduces user input to a supported "major.minor" version:
// "php8.4", "PHP 8.4", "84" and "8.4.7" all become "8.4". Anything that does
// not resolve to a supported version is an error naming the supported list.
func NormalizePHPVersion(input string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(input))
	v = strings.TrimSpace(strings.TrimPrefix(v, "php"))
	if !strings.Contains(v, ".") && len(v) == 2 {
		v = v[:1] + "." + v[1:]
	}
	if parts := strings.SplitN(v, ".", 3); len(parts) >= 2 {
		v = parts[0] + "." + parts[1]
	}
	if !IsSupportedPHPVersion(v) {
		return "", fmt.Errorf("unsupported PHP version %q (supported: %s)", input, strings.Join(SupportedPHPVersions, ", "))
	}
	return v, nil
}

// FrankenPHPVersions returns the subset of SupportedPHPVersions that
// dunglas/frankenphp publishes an image for, in the same ascending order.
func FrankenPHPVersions() []string {
	var out []string
	for _, v := range SupportedPHPVersions {
		if IsFrankenPHPVersion(v) {
			out = append(out, v)
		}
	}
	return out
}

// IsFrankenPHPVersion reports whether dunglas/frankenphp publishes an image for v.
func IsFrankenPHPVersion(v string) bool {
	return IsSupportedPHPVersion(v) && !IsPrereleasePHPVersion(v) &&
		phpVersionAtLeast(v, FrankenPHPMinVersion) && !FrankenPHPTooNew(v)
}

// FrankenPHPTooNew reports whether v is newer than any frankenphp image, so the
// site waits on FPM instead of being told to downgrade.
func FrankenPHPTooNew(v string) bool {
	return IsPrereleasePHPVersion(v) || (v != FrankenPHPMaxVersion && phpVersionAtLeast(v, FrankenPHPMaxVersion))
}

// FrankenPHPUnavailableReason explains why v cannot run under FrankenPHP, or ""
// when it can. Too new gets its own sentence instead of one that reads like the
// user should downgrade.
func FrankenPHPUnavailableReason(v string) string {
	switch {
	case IsFrankenPHPVersion(v):
		return ""
	case FrankenPHPTooNew(v):
		return fmt.Sprintf("dunglas/frankenphp publishes no image for PHP %s yet", v)
	default:
		return fmt.Sprintf("FrankenPHP requires PHP %s or newer; this site is on PHP %s", FrankenPHPMinVersion, v)
	}
}

// LatestFrankenPHPVersion returns the newest PHP version dunglas/frankenphp
// publishes an image for, used as the fallback for unsupported versions. Falls
// back to FrankenPHPMinVersion so callers never index an empty slice.
func LatestFrankenPHPVersion() string {
	v := FrankenPHPVersions()
	if len(v) == 0 {
		return FrankenPHPMinVersion
	}
	return v[len(v)-1]
}

// NormalizeFrankenPHPVersion returns v when dunglas/frankenphp publishes an
// image for it, otherwise the latest version it does, so the base image, the
// derived image name, and the quadlet all agree on one version.
func NormalizeFrankenPHPVersion(v string) string {
	if IsFrankenPHPVersion(v) {
		return v
	}
	return LatestFrankenPHPVersion()
}

// phpVersionAtLeast reports whether "major.minor" version a is >= b.
func phpVersionAtLeast(a, b string) bool {
	amaj, amin := splitMajorMinor(a)
	bmaj, bmin := splitMajorMinor(b)
	if amaj != bmaj {
		return amaj > bmaj
	}
	return amin >= bmin
}

func splitMajorMinor(v string) (int, int) {
	parts := strings.SplitN(v, ".", 2)
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return major, minor
}
