package config

import (
	"strings"
	"testing"
)

func TestIsSupportedPHPVersion(t *testing.T) {
	for _, v := range []string{"7.4", "8.0", "8.5"} {
		if !IsSupportedPHPVersion(v) {
			t.Errorf("IsSupportedPHPVersion(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"5.6", "9.0", ""} {
		if IsSupportedPHPVersion(v) {
			t.Errorf("IsSupportedPHPVersion(%q) = true, want false", v)
		}
	}
}

func TestNormalizePHPVersion(t *testing.T) {
	valid := map[string]string{
		"8.4":     "8.4",
		"php8.4":  "8.4",
		"PHP8.4":  "8.4",
		"php 8.4": "8.4",
		"84":      "8.4",
		"php84":   "8.4",
		"8.4.7":   "8.4",
		" 8.4 ":   "8.4",
		"74":      "7.4",
		"8.0":     "8.0",
	}
	for in, want := range valid {
		got, err := NormalizePHPVersion(in)
		if err != nil {
			t.Errorf("NormalizePHPVersion(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizePHPVersion(%q) = %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"", "php", "9.9", "5.6", "8", "845", "banana", "8.x"} {
		if got, err := NormalizePHPVersion(in); err == nil {
			t.Errorf("NormalizePHPVersion(%q) = %q, want error", in, got)
		} else if !strings.Contains(err.Error(), "supported:") {
			t.Errorf("NormalizePHPVersion(%q) error %q does not list supported versions", in, err)
		}
	}
}

func TestFrankenPHPVersions(t *testing.T) {
	got := FrankenPHPVersions()
	want := []string{"8.2", "8.3", "8.4", "8.5"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("FrankenPHPVersions() = %v, want %v", got, want)
	}
}

func TestIsFrankenPHPVersion(t *testing.T) {
	cases := map[string]bool{
		"7.4": false, "8.0": false, "8.1": false,
		"8.2": true, "8.4": true, "8.5": true,
		"9.0": false, "": false,
	}
	for v, want := range cases {
		if got := IsFrankenPHPVersion(v); got != want {
			t.Errorf("IsFrankenPHPVersion(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestLatestFrankenPHPVersion(t *testing.T) {
	got := LatestFrankenPHPVersion()
	want := FrankenPHPVersions()[len(FrankenPHPVersions())-1]
	if got != want {
		t.Errorf("LatestFrankenPHPVersion() = %q, want %q", got, want)
	}
	if got != "8.5" {
		t.Errorf("LatestFrankenPHPVersion() = %q, want 8.5", got)
	}
}

func TestPHPVersionAtLeast(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"8.2", "8.2", true},
		{"8.5", "8.2", true},
		{"8.1", "8.2", false},
		{"7.4", "8.2", false},
		{"8.10", "8.9", true},
		{"9.0", "8.5", true},
	}
	for _, c := range cases {
		if got := phpVersionAtLeast(c.a, c.b); got != c.want {
			t.Errorf("phpVersionAtLeast(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestPrereleasePHPVersionsAreSupported(t *testing.T) {
	for _, v := range PrereleasePHPVersions {
		if !IsSupportedPHPVersion(v) {
			t.Errorf("prerelease %q is not in SupportedPHPVersions", v)
		}
		if !IsPrereleasePHPVersion(v) {
			t.Errorf("IsPrereleasePHPVersion(%q) = false, want true", v)
		}
	}
	if IsPrereleasePHPVersion("8.5") {
		t.Error("IsPrereleasePHPVersion(8.5) = true, want false")
	}
}

func TestStablePHPVersionsDropsPrereleases(t *testing.T) {
	got := StablePHPVersions()
	for _, v := range got {
		if IsPrereleasePHPVersion(v) {
			t.Errorf("StablePHPVersions() = %v, includes prerelease %q", got, v)
		}
	}
	if len(got)+len(PrereleasePHPVersions) != len(SupportedPHPVersions) {
		t.Errorf("StablePHPVersions() = %v, want every supported version that is not a prerelease", got)
	}
	if got[0] != SupportedPHPVersions[0] {
		t.Errorf("StablePHPVersions() = %v, want the supported order preserved", got)
	}
}

func TestUpstreamPHPTag(t *testing.T) {
	if got := UpstreamPHPTag("8.5"); got != "8.5" {
		t.Errorf("UpstreamPHPTag(8.5) = %q, want 8.5", got)
	}
	for _, v := range PrereleasePHPVersions {
		if got := UpstreamPHPTag(v); got != v+"-rc" {
			t.Errorf("UpstreamPHPTag(%q) = %q, want %q", v, got, v+"-rc")
		}
	}
}

func TestPrereleaseIsNotAFrankenPHPVersion(t *testing.T) {
	for _, v := range PrereleasePHPVersions {
		if IsFrankenPHPVersion(v) {
			t.Errorf("IsFrankenPHPVersion(%q) = true; frankenphp publishes released PHP only", v)
		}
		for _, f := range FrankenPHPVersions() {
			if f == v {
				t.Errorf("FrankenPHPVersions() offers prerelease %q", v)
			}
		}
	}
}

func TestFrankenPHPUnavailableReason(t *testing.T) {
	if got := FrankenPHPUnavailableReason("8.4"); got != "" {
		t.Errorf("FrankenPHPUnavailableReason(8.4) = %q, want empty", got)
	}
	if got := FrankenPHPUnavailableReason("8.1"); !strings.Contains(got, "or newer") {
		t.Errorf("FrankenPHPUnavailableReason(8.1) = %q, want the minimum-version reason", got)
	}
	got := FrankenPHPUnavailableReason("8.6")
	if !strings.Contains(got, "no image for PHP 8.6 yet") {
		t.Errorf("FrankenPHPUnavailableReason(8.6) = %q, want the not-published-yet reason", got)
	}
	if strings.Contains(got, "or newer") || strings.Contains(got, "prerelease") {
		t.Errorf("FrankenPHPUnavailableReason(8.6) = %q, reads like the version is too old or unreleased", got)
	}
}

func TestPHP86IsReleased(t *testing.T) {
	if IsPrereleasePHPVersion("8.6") {
		t.Error("IsPrereleasePHPVersion(8.6) = true, want false since 8.6 shipped")
	}
	if got := UpstreamPHPTag("8.6"); got != "8.6" {
		t.Errorf("UpstreamPHPTag(8.6) = %q, want the released 8.6 tag", got)
	}
	stable := StablePHPVersions()
	if stable[len(stable)-1] != "8.6" {
		t.Errorf("StablePHPVersions() = %v, want 8.6 in a bare fetch", stable)
	}
}

// A released PHP can still be newer than any frankenphp image, so that gate
// stands on its own rather than riding on the prerelease list.
func TestFrankenPHPStopsAtNewestPublished(t *testing.T) {
	if IsFrankenPHPVersion("8.6") {
		t.Error("IsFrankenPHPVersion(8.6) = true; dunglas/frankenphp publishes no 8.6 image")
	}
	if !IsFrankenPHPVersion("8.5") {
		t.Error("IsFrankenPHPVersion(8.5) = false, want true")
	}
	if !FrankenPHPTooNew("8.6") || FrankenPHPTooNew("8.1") || FrankenPHPTooNew("8.5") {
		t.Error("FrankenPHPTooNew must be true only above the newest published image")
	}
	if got := LatestFrankenPHPVersion(); got != "8.5" {
		t.Errorf("LatestFrankenPHPVersion() = %q, want 8.5", got)
	}
}
