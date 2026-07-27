package cli

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestNodeManageDecision(t *testing.T) {
	cases := []struct {
		name        string
		fromUpdate  bool
		saved       *bool
		systemNode  bool
		nvm         bool
		shimPresent bool
		wantWant    bool
		wantPrompt  bool
		wantDefault bool
	}{
		// An explicit saved choice always wins silently, on install and update.
		{"saved opt-out honoured on install", false, boolPtr(false), true, false, true, false, false, false},
		{"saved opt-out honoured on update", true, boolPtr(false), true, false, true, false, false, false},
		{"saved opt-in honoured on install", false, boolPtr(true), false, false, false, true, false, true},
		{"saved opt-in honoured on update", true, boolPtr(true), false, false, false, true, false, true},
		{"saved opt-out honoured with nvm", false, boolPtr(false), true, true, true, false, false, false},

		// No saved choice yet but lerd is already managing (shim present): adopt
		// that as the remembered choice without asking, so an existing user is
		// never re-prompted on a rerun.
		{"install adopts existing managed silently", false, nil, true, false, true, true, false, true},
		{"update adopts existing managed silently", true, nil, true, false, true, true, false, true},

		// No saved choice and no shim on update: preserve the unmanaged state.
		{"update legacy preserves unmanaged", true, nil, true, false, false, false, false, false},
		{"update legacy preserves unmanaged with nvm", true, nil, false, true, false, false, false, false},

		// Genuine first-time install (no saved choice, no shim): prompt when the
		// user already has a Node setup of their own, otherwise default to
		// managed with no prompt. An nvm install counts even with no versions in
		// it yet, which is invisible to the system-node probe.
		{"first install system node prompts", false, nil, true, false, false, false, true, true},
		{"first install nvm without versions prompts", false, nil, false, true, false, false, true, true},
		{"first install system node and nvm prompts", false, nil, true, true, false, false, true, true},
		{"first install no node at all defaults on", false, nil, false, false, false, true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, prompt, def := nodeManageDecision(tc.fromUpdate, tc.saved, tc.systemNode, tc.nvm, tc.shimPresent)
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

func TestNodeStateFlipped(t *testing.T) {
	cases := []struct {
		name        string
		prevManaged bool
		prevManager string
		managed     bool
		manager     string
		want        bool
	}{
		// Existing worker units keep the old manager's exec prefix, so both of
		// these have to regenerate them.
		{"decline flips management", true, "fnm", false, "nvm", true},
		{"accept flips management", false, "nvm", true, "fnm", true},
		{"manager alone flips", true, "fnm", true, "nvm", true},

		// A rerun that answers the same way must not churn the units, and
		// writing "fnm" into a config that predates the setting is not a change.
		{"same answer is not a flip", true, "fnm", true, "fnm", false},
		{"unset manager written as fnm is not a flip", true, "", true, "fnm", false},
		{"unset manager going to nvm is a flip", false, "", false, "nvm", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nodeStateFlipped(tc.prevManaged, tc.prevManager, tc.managed, tc.manager); got != tc.want {
				t.Errorf("nodeStateFlipped(%v, %q, %v, %q) = %v, want %v",
					tc.prevManaged, tc.prevManager, tc.managed, tc.manager, got, tc.want)
			}
		})
	}
}

func TestNodeManagerChoice(t *testing.T) {
	cases := []struct {
		name         string
		saved        string
		wantLerdNode bool
		nvm          bool
		want         string
	}{
		// A saved manager is never revisited, whatever the answers are.
		{"saved nvm survives a managed answer", "nvm", true, true, "nvm"},
		{"saved fnm survives a decline", "fnm", false, true, "fnm"},

		// lerd-managed Node drives the bundled fnm, even when nvm is around:
		// managing means lerd owns the versions, in its own tool.
		{"managed picks fnm", "", true, false, "fnm"},
		{"managed picks fnm despite nvm", "", true, true, "fnm"},

		// Declining hands Node back to the user, so lerd follows their nvm
		// rather than an fnm it will never install a version into.
		{"decline with nvm picks nvm", "", false, true, "nvm"},
		{"decline without nvm keeps fnm", "", false, false, "fnm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nodeManagerChoice(tc.saved, tc.wantLerdNode, tc.nvm); got != tc.want {
				t.Errorf("nodeManagerChoice(%q, %v, %v) = %q, want %q", tc.saved, tc.wantLerdNode, tc.nvm, got, tc.want)
			}
		})
	}
}
