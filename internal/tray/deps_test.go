package tray

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingFrom(t *testing.T) {
	index := map[string]struct{}{
		"libc.so.6":   {},
		"libgtk-3.so": {},
	}

	tests := []struct {
		name   string
		needed []string
		want   []string
	}{
		{
			name:   "atomic image without the appindicator library",
			needed: []string{"libayatana-appindicator3.so.1", "libc.so.6"},
			want:   []string{"libayatana-appindicator3.so.1"},
		},
		{
			name:   "everything resolves",
			needed: []string{"libc.so.6", "libgtk-3.so"},
		},
		{
			name:   "nothing needed",
			needed: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := missingFrom(tt.needed, index)
			if len(got) != len(tt.want) {
				t.Fatalf("missingFrom() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("missingFrom()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// A probe that cannot read the helper must not claim libraries are missing:
// reporting a false positive would disable a perfectly good tray.
func TestMissingLibsIsConservative(t *testing.T) {
	if got := MissingLibs(filepath.Join(t.TempDir(), "no-such-helper")); got != nil {
		t.Errorf("MissingLibs(absent binary) = %v, want nil", got)
	}

	notELF := filepath.Join(t.TempDir(), "lerd-tray")
	if err := os.WriteFile(notELF, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := MissingLibs(notELF); got != nil {
		t.Errorf("MissingLibs(non-ELF) = %v, want nil", got)
	}
}

// The running test binary is by definition fully resolvable, so a real ELF read
// through the real host index must come back clean.
func TestMissingLibsOnResolvableBinary(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip("cannot locate test binary")
	}
	if got := MissingLibs(self); got != nil {
		t.Errorf("MissingLibs(self) = %v, want nil — the running binary resolves", got)
	}
}
