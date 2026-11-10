package cli

import (
	"errors"
	"testing"
)

func TestOfferGlobalMCP(t *testing.T) {
	cases := []struct {
		name        string
		enabled     bool
		declined    bool
		unattended  bool
		terminal    bool
		answer      bool
		enableErr   error
		wantAsked   bool
		wantEnabled bool
		wantSaved   bool
	}{
		{"already enabled is never asked", true, false, false, true, true, nil, false, false, false},
		{"an earlier no is never asked again", false, true, false, true, true, nil, false, false, false},
		{"unattended never asks", false, false, true, true, true, nil, false, false, false},
		{"no terminal never asks", false, false, false, false, true, nil, false, false, false},
		{"yes runs enable-global", false, false, false, true, true, nil, true, true, false},
		{"no is remembered", false, false, false, true, false, nil, true, false, true},
		{"enable failure does not abort install", false, false, false, true, true, errors.New("boom"), true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asked, enabled, saved := false, false, false
			offerGlobalMCP(tc.unattended, globalMCPOffer{
				enabled:  func() bool { return tc.enabled },
				declined: func() bool { return tc.declined },
				decline:  func() { saved = true },
				terminal: func() bool { return tc.terminal },
				ask:      func(string) bool { asked = true; return tc.answer },
				enable:   func() error { enabled = true; return tc.enableErr },
			})
			if asked != tc.wantAsked {
				t.Errorf("asked = %v, want %v", asked, tc.wantAsked)
			}
			if enabled != tc.wantEnabled {
				t.Errorf("enable ran = %v, want %v", enabled, tc.wantEnabled)
			}
			if saved != tc.wantSaved {
				t.Errorf("decline saved = %v, want %v", saved, tc.wantSaved)
			}
		})
	}
}

func TestGlobalMCPDeclineIsRemembered(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	o := defaultGlobalMCPOffer()
	if o.declined() {
		t.Fatal("fresh config reports a decline")
	}
	o.decline()
	if !defaultGlobalMCPOffer().declined() {
		t.Fatal("decline was not persisted")
	}
}
