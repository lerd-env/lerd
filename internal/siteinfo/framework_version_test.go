package siteinfo

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The label and the version a surface shows are the project's, not the
// definition's: a WordPress 7 site served by the 6 definition read "WordPress 6"
// and looked a major version behind.
func TestFrameworkVersionOf_prefersWhatTheProjectRuns(t *testing.T) {
	borrowed := &config.Framework{Label: "WordPress", Version: "6", DetectedVersion: "7"}
	if got := frameworkVersionOf(borrowed); got != "7" {
		t.Errorf("frameworkVersionOf = %q, want the project's 7", got)
	}
	if got := frameworkLabel("wordpress", "/srv/blog", borrowed, true); got != "WordPress 7" {
		t.Errorf("label = %q, want WordPress 7", got)
	}
}

// A legacy project reports its own version too, which it already did, and this
// must not regress while the flag stops being what carries it.
func TestFrameworkVersionOf_legacyProjectUnchanged(t *testing.T) {
	clamped := &config.Framework{Label: "Laravel", Version: "10", DetectedVersion: "6", VersionGuessed: true}
	if got := frameworkLabel("laravel", "/srv/app", clamped, true); got != "Laravel 6" {
		t.Errorf("label = %q, want Laravel 6", got)
	}
}

// An exact match has only one version to report.
func TestFrameworkVersionOf_exactMatch(t *testing.T) {
	exact := &config.Framework{Label: "Laravel", Version: "12"}
	if got := frameworkLabel("laravel", "/srv/app", exact, true); got != "Laravel 12" {
		t.Errorf("label = %q, want Laravel 12", got)
	}
	if got := frameworkVersionOf(nil); got != "" {
		t.Errorf("frameworkVersionOf(nil) = %q, want empty", got)
	}
}
