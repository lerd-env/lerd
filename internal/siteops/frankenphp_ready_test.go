package siteops

import (
	"errors"
	"strings"
	"testing"
)

// A runtime switch that cannot run must fail loudly. Reporting success while the
// image is missing leaves the unit restart-looping against an image podman then
// tries to pull from a registry called "localhost", and the site sits on 502
// with nothing saying why.
func TestFrankenPHPPreflight(t *testing.T) {
	t.Run("missing image is fatal and names the version", func(t *testing.T) {
		err := frankenPHPPreflight("8.5", errors.New("build failed"), false)
		if err == nil {
			t.Fatal("a missing image was allowed through")
		}
		if !strings.Contains(err.Error(), "8.5") {
			t.Errorf("error should name the PHP version, got: %v", err)
		}
	})

	t.Run("a build error is tolerated when the image is already there", func(t *testing.T) {
		if err := frankenPHPPreflight("8.5", errors.New("build failed"), true); err != nil {
			t.Errorf("an existing image should survive a failed rebuild: %v", err)
		}
	})

	t.Run("clean build passes", func(t *testing.T) {
		if err := frankenPHPPreflight("8.5", nil, true); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// systemctl start returns before a quadlet unit has settled, so the switch has
// to check what the unit actually did rather than trusting the start call.
func TestFrankenPHPUnitFailure(t *testing.T) {
	err := frankenPHPUnitError("lerd-fp-demo3", false)
	if err == nil {
		t.Fatal("a failed unit was reported as a successful switch")
	}
	if !strings.Contains(err.Error(), "lerd-fp-demo3") {
		t.Errorf("error should name the unit so it can be inspected, got: %v", err)
	}
	if err := frankenPHPUnitError("lerd-fp-demo3", true); err != nil {
		t.Errorf("a running unit should not error: %v", err)
	}
}
