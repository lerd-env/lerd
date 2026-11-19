package podman

import (
	"os/exec"
	"testing"
	"time"
)

func captureAnnouncements(t *testing.T) *[]string {
	t.Helper()
	var got []string
	SetOnBackgroundRead(func(unit string) { got = append(got, unit) })
	t.Cleanup(func() { SetOnBackgroundRead(nil) })
	return &got
}

// The PHP patch is probed in the background and lerd-ui caches what the status
// showed, so a probe that learns a new patch has to say so, and one that finds
// nothing new must stay quiet or every refill would trigger another.
func TestPHPVersionProbeAnnouncesOnlyANewPatch(t *testing.T) {
	origID, origExec := imageIDFn, execCommand
	t.Cleanup(func() { imageIDFn = origID; execCommand = origExec; resetPHPVerCache() })
	resetPHPVerCache()
	got := captureAnnouncements(t)

	imageIDFn = func(string) string { return "sha256:aaa" }
	execCommand = func(string, ...string) *exec.Cmd { return exec.Command("printf", "8.4.1") }
	refreshPHPVersion("8.4")
	refreshPHPVersion("8.4")

	if len(*got) != 1 || (*got)[0] != "lerd-php84-fpm" {
		t.Errorf("announced %v, want [lerd-php84-fpm] once", *got)
	}
}

// A cold base digest is fetched in the background; landing it must reach
// lerd-ui, or the cached "no update" stands until the hourly clear.
func TestBaseDigestRefreshAnnouncesTheVersion(t *testing.T) {
	prev := manifestDigestFn
	t.Cleanup(func() { manifestDigestFn = prev })
	manifestDigestFn = func(string) (string, error) { return "sha256:base", nil }
	done := make(chan string, 1)
	SetOnBackgroundRead(func(unit string) { done <- unit })
	t.Cleanup(func() { SetOnBackgroundRead(nil) })

	scheduleBaseDigestRefresh("8.4")
	select {
	case unit := <-done:
		if unit != "lerd-php84-fpm" {
			t.Errorf("announced %q, want lerd-php84-fpm", unit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a finished base digest fetch did not announce itself")
	}
}
