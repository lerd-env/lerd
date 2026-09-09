package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestPHPRuntimeFromArgs(t *testing.T) {
	if _, show, err := phpRuntimeFromArgs(nil); err != nil || !show {
		t.Errorf("no argument should mean show, got show=%v err=%v", show, err)
	}
	for _, mode := range []string{config.PHPRuntimeNative, config.PHPRuntimeContainer} {
		got, show, err := phpRuntimeFromArgs([]string{mode})
		if err != nil || show || got != mode {
			t.Errorf("%q: got (%q,%v,%v)", mode, got, show, err)
		}
	}
	if _, _, err := phpRuntimeFromArgs([]string{"nonsense"}); err == nil {
		t.Error("an unknown mode should be rejected, not silently normalised")
	}
}

// Going native leaves nothing for the shared FPM containers to serve, and on a
// 4GB VM that is memory worth reclaiming. Only the versions lerd actually runs
// a container for are touched.
func TestFPMUnitsToStop(t *testing.T) {
	got := fpmUnitsFor([]string{"8.4", "8.5"})
	want := map[string]bool{"lerd-php84-fpm": true, "lerd-php85-fpm": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want two units", got)
	}
	for _, u := range got {
		if !want[u] {
			t.Errorf("unexpected unit %q", u)
		}
	}
	if len(fpmUnitsFor(nil)) != 0 {
		t.Error("no versions should mean no units")
	}
}
