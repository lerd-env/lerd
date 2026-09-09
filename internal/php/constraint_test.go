package php

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// installedPHP stages quadlets so ListInstalled reports exactly these versions,
// which is what "the best installed version that satisfies" resolves against.
// Without this the answer depends on whatever the developer happens to have.
func installedPHP(t *testing.T, versions ...string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	// macOS reads the installed set from ~/Library/LaunchAgents, so the real
	// home would add whatever the developer has installed to the staged set.
	t.Setenv("HOME", t.TempDir())
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, v := range versions {
		short := ""
		for _, c := range v {
			if c != '.' {
				short += string(c)
			}
		}
		name := filepath.Join(dir, "lerd-php"+short+"-fpm.container")
		if err := os.WriteFile(name, []byte("[Container]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClampToConstraint(t *testing.T) {
	cases := []struct {
		name, version, constraint, want string
		installed                       []string
	}{
		// An old project allows 7.4, and nothing may talk it out of that.
		{name: "in range via alternation", version: "7.4", constraint: "^7.3|^8.0", want: "7.4", installed: []string{"7.4", "8.5"}},
		{name: "in range simple", version: "8.3", constraint: "^8.2", want: "8.3", installed: []string{"8.3", "8.5"}},
		// Out of range moves to the best installed that satisfies, not to the
		// literal minimum, so a machine with 8.4 does not get sent to 8.2.
		{name: "below minimum picks best installed", version: "7.4", constraint: "^8.2", want: "8.4", installed: []string{"8.2", "8.4"}},
		// Nothing installed satisfies: the constraint's own minimum is the honest
		// answer, and the caller reports the image gap.
		{name: "nothing installed satisfies", version: "7.4", constraint: "^8.2", want: "8.2", installed: []string{"7.4"}},
		{name: "no constraint leaves it alone", version: "7.4", constraint: "", want: "7.4"},
		{name: "unparseable leaves it alone", version: "7.4", constraint: "not-a-constraint", want: "7.4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installedPHP(t, tc.installed...)
			if got := ClampToConstraint(tc.version, tc.constraint); got != tc.want {
				t.Errorf("ClampToConstraint(%q, %q) = %q, want %q", tc.version, tc.constraint, got, tc.want)
			}
		})
	}
}

func TestComposerPHPConstraint(t *testing.T) {
	dir := t.TempDir()
	if got := ComposerPHPConstraint(dir); got != "" {
		t.Errorf("no composer.json should yield no constraint, got %q", got)
	}
	writeFile(t, dir, "composer.json", `{"require":{"php":"^7.3|^8.0","laravel/framework":"^8.0"}}`)
	if got := ComposerPHPConstraint(dir); got != "^7.3|^8.0" {
		t.Errorf("ComposerPHPConstraint = %q, want ^7.3|^8.0", got)
	}
}

// Composer accepts both spellings of OR, and the projects this matters most for
// use the single pipe: Laravel 8 requires "^7.3|^8.0". Reading that as one term
// meant nothing satisfied it, so detection fell through to the constraint's
// minimum and pinned 7.3 on a machine running 8.5.
func TestSatisfiesConstraint_BothSpellingsOfOr(t *testing.T) {
	for _, c := range []struct {
		version, constraint string
		want                bool
	}{
		{"8.5", "^7.3||^8.0", true},
		{"8.5", "^7.3|^8.0", true},
		{"8.5", "^7.3 | ^8.0", true},
		{"7.4", "^7.3|^8.0", true},
		{"7.2", "^7.3|^8.0", false},
		{"9.0", "^7.3|^8.0", false},
	} {
		if got := satisfiesConstraint(c.version, c.constraint); got != c.want {
			t.Errorf("satisfiesConstraint(%q, %q) = %v, want %v", c.version, c.constraint, got, c.want)
		}
	}
}

// A framework definition's range describes the framework; composer.json
// describes what this project actually boots on. Where they disagree the
// project has to win, so the answer needs to be checkable on its own.
func TestSatisfies(t *testing.T) {
	cases := []struct {
		version    string
		constraint string
		want       bool
	}{
		{"8.1", ">=8.1", true},
		{"8.0", ">=8.1", false},
		{"8.1", "^7.3|^8.0", true},
		{"8.5", "", true},
	}
	for _, c := range cases {
		if got := Satisfies(c.version, c.constraint); got != c.want {
			t.Errorf("Satisfies(%q, %q) = %v, want %v", c.version, c.constraint, got, c.want)
		}
	}
}

// A project is governed by more than one constraint at a time: the framework
// definition's range and its own composer requirement. Both have to hold.
func TestSatisfiesAll(t *testing.T) {
	cases := []struct {
		version     string
		constraints []string
		want        bool
	}{
		{"8.1", []string{">=8.0 <=8.2", ">=8.1"}, true},
		{"8.0", []string{">=8.0 <=8.2", ">=8.1"}, false},
		{"8.5", []string{">=8.0 <=8.2", ">=8.1"}, false},
		{"8.4", []string{"", ">=8.1"}, true},
		{"8.4", nil, true},
	}
	for _, c := range cases {
		if got := SatisfiesAll(c.version, c.constraints...); got != c.want {
			t.Errorf("SatisfiesAll(%q, %v) = %v, want %v", c.version, c.constraints, got, c.want)
		}
	}
}

func TestBestInstalledFor(t *testing.T) {
	installedPHP(t, "8.0", "8.1", "8.4", "8.5")

	if got := BestInstalledFor(">=8.0 <=8.2", ">=8.1"); got != "8.1" {
		t.Errorf("best = %q, want 8.1", got)
	}
	if got := BestInstalledFor(">=8.0 <=8.2", ">=8.4"); got != "" {
		t.Errorf("best = %q, want nothing installed to satisfy both", got)
	}
	if got := BestInstalledFor("", ">=8.4"); got != "8.5" {
		t.Errorf("best = %q, want 8.5", got)
	}
}

// Whether two constraints can both be met is a property of the constraints, not
// of this machine: a fresh install with no PHP built yet must reach the same
// answer as a developer's box with five versions on it.
func TestConstraintsOverlap(t *testing.T) {
	installedPHP(t) // nothing installed at all

	if !ConstraintsOverlap(">=8.0 <=8.2", ">=8.1") {
		t.Error("overlap = false, want 8.1 and 8.2 to satisfy both")
	}
	if ConstraintsOverlap(">=8.0 <=8.2", ">=8.4") {
		t.Error("overlap = true, want no version to satisfy both")
	}
	if !ConstraintsOverlap(">=8.3 <=8.5", "") {
		t.Error("overlap = false, want an empty constraint to constrain nothing")
	}
}
