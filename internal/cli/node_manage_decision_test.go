package cli

import (
	"strings"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestNodeManageDecision(t *testing.T) {
	cases := []struct {
		name        string
		fromUpdate  bool
		unattended  bool
		saved       *bool
		systemNode  bool
		nvm         bool
		shimPresent bool
		wantWant    bool
		wantPrompt  bool
		wantDefault bool
	}{
		// An explicit saved choice always wins silently, on install and update.
		{"saved opt-out honoured on install", false, false, boolPtr(false), true, false, true, false, false, false},
		{"saved opt-out honoured on update", true, false, boolPtr(false), true, false, true, false, false, false},
		{"saved opt-in honoured on install", false, false, boolPtr(true), false, false, false, true, false, true},
		{"saved opt-in honoured on update", true, false, boolPtr(true), false, false, false, true, false, true},
		{"saved opt-out honoured with nvm", false, false, boolPtr(false), true, true, true, false, false, false},

		// No saved choice yet but lerd is already managing (shim present): adopt
		// that as the remembered choice without asking, so an existing user is
		// never re-prompted on a rerun.
		{"install adopts existing managed silently", false, false, nil, true, false, true, true, false, true},
		{"update adopts existing managed silently", true, false, nil, true, false, true, true, false, true},

		// No saved choice and no shim on update: preserve the unmanaged state.
		{"update legacy preserves unmanaged", true, false, nil, true, false, false, false, false, false},
		{"update legacy preserves unmanaged with nvm", true, false, nil, false, true, false, false, false, false},

		// Genuine first-time install (no saved choice, no shim): prompt when the
		// user already has a Node setup of their own, otherwise default to
		// managed with no prompt. An nvm install counts even with no versions in
		// it yet, which is invisible to the system-node probe.
		{"first install system node prompts", false, false, nil, true, false, false, false, true, true},
		{"first install nvm without versions prompts", false, false, nil, false, true, false, false, true, true},
		{"first install system node and nvm prompts", false, false, nil, true, true, false, false, true, true},
		{"first install no node at all defaults on", false, false, nil, false, false, false, true, false, true},

		// A package install has nobody to ask, but must still leave a working Node
		// behind: an existing nvm drives it, otherwise lerd owns it through mise.
		{"unattended without nvm manages node", true, true, nil, false, false, false, true, false, true},
		{"unattended with nvm defers to nvm", true, true, nil, false, true, false, false, false, false},
		{"unattended with system node but no nvm still manages", true, true, nil, true, false, false, true, false, true},
		{"unattended never prompts", true, true, nil, true, true, false, false, false, false},

		// An unattended rerun over an existing managed setup keeps it, and a
		// saved choice still outranks everything.
		{"unattended keeps existing managed", true, true, nil, false, false, true, true, false, true},
		{"unattended honours a saved opt-out", true, true, boolPtr(false), false, false, false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, prompt, def := nodeManageDecision(tc.fromUpdate, tc.unattended, tc.saved, tc.systemNode, tc.nvm, tc.shimPresent)
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

func TestNodeManagerChoice(t *testing.T) {
	cases := []struct {
		name         string
		saved        string
		wantLerdNode bool
		userManager  string
		fnmPresent   bool
		fnmIsUsers   bool
		want         string
	}{
		// A saved manager is never revisited, whatever the answers are.
		{"saved nvm survives a managed answer", "nvm", true, "nvm", false, false, "nvm"},
		{"saved fnm survives a decline", "fnm", false, "nvm", true, false, "fnm"},
		{"saved fnm of the user's survives", "fnm", true, "fnm", true, true, "fnm"},

		// lerd no longer installs fnm, so one that is gone is not fetched again.
		{"saved fnm that is gone moves to mise", "fnm", true, "", false, false, "mise"},
		{"saved fnm that is gone follows nvm on a decline", "fnm", false, "nvm", false, false, "nvm"},

		// lerd-managed Node drives mise, even next to the user's own manager:
		// managing means lerd owns the versions, in its own tool.
		{"managed picks mise", "", true, "", false, false, "mise"},
		{"managed picks mise despite nvm", "", true, "nvm", false, false, "mise"},
		{"managed picks mise despite the user's fnm", "", true, "fnm", true, true, "mise"},

		// Declining hands Node back to the user, so lerd follows their manager
		// rather than one it will never install a version into.
		{"decline with nvm picks nvm", "", false, "nvm", false, false, "nvm"},
		{"decline with the user's fnm picks fnm", "", false, "fnm", true, true, "fnm"},
		{"decline with neither keeps mise", "", false, "", false, false, "mise"},

		// An fnm in lerd's bin dir with no manager in config is an install from
		// before the setting existed: it was driving fnm and keeps driving it,
		// rather than being moved to mise behind the user's back by an update.
		{"an existing fnm install stays on fnm", "", true, "", true, false, "fnm"},
		{"an existing fnm install stays on fnm despite nvm", "", true, "nvm", true, false, "fnm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nodeManagerChoice(tc.saved, tc.wantLerdNode, tc.userManager, tc.fnmPresent, tc.fnmIsUsers); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNodeManagerNotice(t *testing.T) {
	if got := nodeManagerNotice("fnm", "nvm"); got != "" {
		t.Errorf("an fnm kept from an older install should print nothing, got %q", got)
	}
	for _, user := range []string{"nvm", "fnm"} {
		if got := nodeManagerNotice("mise", user); !strings.Contains(got, "using mise") || !strings.HasSuffix(got, user) {
			t.Errorf("mise notice next to %s = %q", user, got)
		}
		if got := nodeManagerNotice(user, user); !strings.Contains(got, "leaving Node to your "+user) {
			t.Errorf("%s notice = %q", user, got)
		}
	}
}
