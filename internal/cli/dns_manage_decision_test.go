package cli

import "testing"

func dnsBoolPtr(b bool) *bool { return &b }

func TestDNSManageDecision(t *testing.T) {
	cases := []struct {
		name          string
		fromUpdate    bool
		configExisted bool
		flag          *bool
		savedEnabled  bool
		wantWant      bool
		wantPrompt    bool
	}{
		// The --dns flag always wins, no prompt, on any run.
		{"flag managed wins", false, false, dnsBoolPtr(true), false, true, false},
		{"flag localhost wins over existing config", false, true, dnsBoolPtr(false), true, false, false},

		// Update honours the saved choice silently.
		{"update honours saved enabled", true, true, nil, true, true, false},
		{"update honours saved disabled", true, true, nil, false, false, false},

		// A rerun over an existing config never re-prompts, it honours the saved
		// choice, so a prior dns:disable is not undone.
		{"rerun honours saved enabled", false, true, nil, true, true, false},
		{"rerun honours saved disabled", false, true, nil, false, false, false},

		// Only a genuine first install (no config file) asks, defaulting to managed.
		{"first install asks", false, false, nil, true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, prompt := dnsManageDecision(tc.fromUpdate, tc.configExisted, tc.flag, tc.savedEnabled)
			if prompt != tc.wantPrompt {
				t.Errorf("needPrompt = %v, want %v", prompt, tc.wantPrompt)
			}
			// want is only meaningful when the caller does not prompt.
			if !prompt && want != tc.wantWant {
				t.Errorf("want = %v, want %v", want, tc.wantWant)
			}
		})
	}
}

func TestToggledCanonicalTLD(t *testing.T) {
	cases := []struct {
		prevTLD  string
		enabling bool
		want     string
	}{
		{"localhost", true, "test"},       // enabling flips the localhost default to test
		{"test", false, "localhost"},      // disabling flips the test default to localhost
		{"test", true, "test"},            // already canonical for enabled, unchanged
		{"localhost", false, "localhost"}, // already canonical for disabled, unchanged
		{"dev", true, "dev"},              // a custom TLD is preserved
		{"dev", false, "dev"},             // a custom TLD is preserved
	}
	for _, tc := range cases {
		if got := toggledCanonicalTLD(tc.prevTLD, tc.enabling); got != tc.want {
			t.Errorf("toggledCanonicalTLD(%q, %v) = %q, want %q", tc.prevTLD, tc.enabling, got, tc.want)
		}
	}
}
