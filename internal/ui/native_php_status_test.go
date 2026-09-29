package ui

import "testing"

// On the native runtime a version's patch is the build installed on the host,
// and an update is a newer build being published. Reported from the image and
// its base before this, which under that runtime describes something that is
// not serving anything.
func TestNativePHPStatus(t *testing.T) {
	const (
		older = "2026-09-21T06:00:00Z"
		newer = "2026-09-28T12:00:00Z"
	)
	cases := []struct {
		name                            string
		installed, pinned               string
		publishedAt, installedPublished string
		wantPatch                       string
		wantUpdate                      bool
	}{
		{"current", "8.4.25", "8.4.25", "", "", "8.4.25", false},
		{"a newer build is published", "8.4.24", "8.4.25", "", "", "8.4.24", true},
		// No stamp is a build installed before lerd recorded patches. It is
		// serving, so it is not an update waiting to happen.
		{"no stamp", "", "8.4.25", "", "", "", false},
		// Nothing published to compare against, so nothing to offer.
		{"no pin", "8.4.25", "", "", "", "8.4.25", false},
		// The patch stands still while the build behind it is replaced, which
		// is what a collector fix produces. The card has to see that too, or
		// the update it offers is the only one the CLI would take.
		{"same patch rebuilt", "8.4.25", "8.4.25", newer, older, "8.4.25", true},
		{"same build", "8.4.25", "8.4.25", newer, newer, "8.4.25", false},
		{"installed predates the date", "8.4.25", "8.4.25", newer, "", "8.4.25", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			patch, update := nativePHPStatus(c.installed, c.pinned, c.publishedAt, c.installedPublished)
			if patch != c.wantPatch || update != c.wantUpdate {
				t.Errorf("nativePHPStatus(%q,%q,%q,%q) = (%q,%v), want (%q,%v)",
					c.installed, c.pinned, c.publishedAt, c.installedPublished, patch, update, c.wantPatch, c.wantUpdate)
			}
		})
	}
}
