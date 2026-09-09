package ui

import "testing"

// On the native runtime a version's patch is the build installed on the host,
// and an update is a newer build being published. Reported from the image and
// its base before this, which under that runtime describes something that is
// not serving anything.
func TestNativePHPStatus(t *testing.T) {
	cases := []struct {
		name              string
		installed, pinned string
		wantPatch         string
		wantUpdate        bool
	}{
		{"current", "8.4.25", "8.4.25", "8.4.25", false},
		{"a newer build is published", "8.4.24", "8.4.25", "8.4.24", true},
		// No stamp is a build installed before lerd recorded patches. It is
		// serving, so it is not an update waiting to happen.
		{"no stamp", "", "8.4.25", "", false},
		// Nothing published to compare against, so nothing to offer.
		{"no pin", "8.4.25", "", "8.4.25", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			patch, update := nativePHPStatus(c.installed, c.pinned)
			if patch != c.wantPatch || update != c.wantUpdate {
				t.Errorf("nativePHPStatus(%q,%q) = (%q,%v), want (%q,%v)",
					c.installed, c.pinned, patch, update, c.wantPatch, c.wantUpdate)
			}
		})
	}
}
