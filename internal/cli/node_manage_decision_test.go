package cli

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestNodeManageDecision(t *testing.T) {
	cases := []struct {
		name        string
		fromUpdate  bool
		saved       *bool
		systemNode  bool
		shimPresent bool
		wantWant    bool
		wantPrompt  bool
		wantDefault bool
	}{
		// An explicit saved choice always wins silently, on install and update.
		{"saved opt-out honoured on install", false, boolPtr(false), true, true, false, false, false},
		{"saved opt-out honoured on update", true, boolPtr(false), true, true, false, false, false},
		{"saved opt-in honoured on install", false, boolPtr(true), false, false, true, false, true},
		{"saved opt-in honoured on update", true, boolPtr(true), false, false, true, false, true},

		// No saved choice yet but lerd is already managing (shim present): adopt
		// that as the remembered choice without asking, so an existing user is
		// never re-prompted on a rerun.
		{"install adopts existing managed silently", false, nil, true, true, true, false, true},
		{"update adopts existing managed silently", true, nil, true, true, true, false, true},

		// No saved choice and no shim on update: preserve the unmanaged state.
		{"update legacy preserves unmanaged", true, nil, true, false, false, false, false},

		// Genuine first-time install (no saved choice, no shim): prompt only when
		// a system node exists, otherwise default to managed with no prompt.
		{"first install system node prompts", false, nil, true, false, false, true, true},
		{"first install no system node defaults on", false, nil, false, false, true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, prompt, def := nodeManageDecision(tc.fromUpdate, tc.saved, tc.systemNode, tc.shimPresent)
			if prompt != tc.wantPrompt {
				t.Errorf("needPrompt = %v, want %v", prompt, tc.wantPrompt)
			}
			if def != tc.wantDefault {
				t.Errorf("promptDefault = %v, want %v", def, tc.wantDefault)
			}
			// want is only meaningful when the caller does not prompt; when it
			// prompts it overrides want with the answer, so only assert otherwise.
			if !prompt && want != tc.wantWant {
				t.Errorf("want = %v, want %v", want, tc.wantWant)
			}
		})
	}
}
