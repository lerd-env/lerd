package cli

import (
	"strings"
	"testing"
)

// Under the native runtime there is no image to inspect, and the fix doctor
// points at for a stale one refuses outright. What can be wrong instead is a
// build that is absent or behind the published patch.
func TestNativeBuildFinding(t *testing.T) {
	cases := []struct {
		name               string
		present            bool
		installed, pinned  string
		wantStatus, wantIn string
	}{
		{"up to date", true, "8.4.25", "8.4.25", "ok", ""},
		{"a newer patch is published", true, "8.4.24", "8.4.25", "warn", "8.4.25"},
		{"absent", false, "", "8.4.25", "fail", "not installed"},
		// With no pin reachable there is nothing to compare against, and an
		// installed build is not suspect just because the network is down.
		{"no pin available", true, "8.4.25", "", "ok", ""},
		// A binary installed before lerd recorded patches has no stamp. It is
		// on disk and serving, so it is not missing and there is nothing to
		// compare it against either.
		{"present but unstamped", true, "", "8.4.25", "ok", ""},
		// Nothing is published for a version like a prerelease, so naming an
		// install command would send the reader after a build that is not there.
		{"no build published", false, "", "", "fail", "no native build"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, detail := nativeBuildFinding(c.present, c.installed, c.pinned)
			if status != c.wantStatus {
				t.Errorf("status = %q, want %q (detail %q)", status, c.wantStatus, detail)
			}
			if c.wantIn != "" && !strings.Contains(detail, c.wantIn) {
				t.Errorf("detail %q should mention %q", detail, c.wantIn)
			}
		})
	}
}
