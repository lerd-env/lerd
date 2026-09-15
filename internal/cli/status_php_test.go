package cli

import (
	"strings"
	"testing"
)

// Under the native runtime there is no image to look for and no container to
// start, so a status built from those reported every version as broken while
// the host pools were serving.
func TestNativePHPStatusReportsThePools(t *testing.T) {
	out := captureStdout(t, func() {
		printNativePHPStatus([]string{"8.2", "8.4"}, func(v string) bool { return v == "8.4" })
	})

	if !strings.Contains(out, "PHP 8.4") {
		t.Errorf("out = %q, want a line for the running pool", out)
	}
	if !strings.Contains(out, "pool not running") {
		t.Errorf("out = %q, want the stopped pool named as a pool", out)
	}
	for _, unwanted := range []string{"image missing", "php:rebuild", "container"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("out = %q, want nothing naming container work", out)
		}
	}
}

func TestNativePHPStatusWithNothingInstalled(t *testing.T) {
	out := captureStdout(t, func() {
		printNativePHPStatus(nil, func(string) bool { return true })
	})
	if !strings.Contains(out, "none installed") {
		t.Errorf("out = %q, want the empty case reported", out)
	}
}

// A stopped pool is started by bringing lerd up, not by rebuilding an image.
func TestNativePHPStatusHintsAtStart(t *testing.T) {
	out := captureStdout(t, func() {
		printNativePHPStatus([]string{"8.2"}, func(string) bool { return false })
	})
	if !strings.Contains(out, "lerd start") {
		t.Errorf("out = %q, want the hint to name lerd start", out)
	}
}
