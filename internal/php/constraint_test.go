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

// Composer's hyphen range ("8.2 - 8.5", as the Yii 3 app template writes it)
// was split on its spaces into three exact matches, so nothing satisfied it.
func TestSatisfiesConstraint_HyphenRange(t *testing.T) {
	for _, c := range []struct {
		version, constraint string
		want                bool
	}{
		{"8.2", "8.2 - 8.5", true},
		{"8.4", "8.2 - 8.5", true},
		{"8.5", "8.2 - 8.5", true},
		{"8.1", "8.2 - 8.5", false},
		{"8.6", "8.2 - 8.5", false},
		{"8.5", "8.2 - 8.5.3", true},
		{"8.4", "^7.4 || 8.2 - 8.4", true},
	} {
		if got := satisfiesConstraint(c.version, c.constraint); got != c.want {
			t.Errorf("satisfiesConstraint(%q, %q) = %v, want %v", c.version, c.constraint, got, c.want)
		}
	}
}
