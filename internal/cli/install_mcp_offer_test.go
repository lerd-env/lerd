package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestOfferGlobalMCP(t *testing.T) {
	cases := []struct {
		name        string
		enabled     bool
		declined    bool
		unattended  bool
		answered    bool
		answer      bool
		enableErr   error
		wantAsked   bool
		wantEnabled bool
		wantSaved   bool
	}{
		{"already enabled is never asked", true, false, false, true, true, nil, false, false, false},
		{"an earlier no is never asked again", false, true, false, true, true, nil, false, false, false},
		{"unattended never asks", false, false, true, true, true, nil, false, false, false},
		{"no answer, from no terminal or EOF, changes nothing", false, false, false, false, true, nil, true, false, false},
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
				ask:      func(string) (bool, bool) { asked = true; return tc.answer, tc.answered },
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

func TestReadGlobalMCPAnswer(t *testing.T) {
	cases := []struct {
		in                    string
		wantYes, wantAnswered bool
	}{
		{"", false, false},
		{"y", false, false},
		{"\n", true, true},
		{"y\n", true, true},
		{"n\n", false, true},
		{"No\n", false, true},
	}
	for _, tc := range cases {
		yes, answered := readGlobalMCPAnswer(strings.NewReader(tc.in))
		if yes != tc.wantYes || answered != tc.wantAnswered {
			t.Errorf("%q: got yes=%v answered=%v, want yes=%v answered=%v", tc.in, yes, answered, tc.wantYes, tc.wantAnswered)
		}
	}
}
