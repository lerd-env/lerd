package php

import "testing"

// A site with nothing but composer.json to go on used to move to every newer
// PHP its constraint allowed as soon as one was installed (#2667). Detection
// for a site already being served keeps its version while that still fits.
func TestDetectVersionKeeping(t *testing.T) {
	cases := []struct {
		name      string
		installed []string
		composer  string
		pin       string
		current   string
		want      string
	}{
		{"a newer allowed version does not move the site", []string{"8.4", "8.5", "8.6"}, "^8.4", "", "8.4", "8.4"},
		{"an explicit pin still moves it", []string{"8.4", "8.5"}, "^8.4", "8.5", "8.4", "8.5"},
		{"a constraint that rules the version out moves it", []string{"8.3", "8.4", "8.5"}, "^8.4", "", "8.3", "8.5"},
		{"an uninstalled version moves it", []string{"8.5", "8.6"}, "^8.4", "", "8.4", "8.6"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			t.Setenv("HOME", t.TempDir())
			stubRuntimeInstalled(t, false, nil, c.installed)
			dir := t.TempDir()
			writeFile(t, dir, "composer.json", `{"require":{"php":"`+c.composer+`"}}`)
			if c.pin != "" {
				writeFile(t, dir, ".php-version", c.pin+"\n")
			}
			if got := DetectVersionKeeping(dir, "", "", c.current); got != c.want {
				t.Errorf("DetectVersionKeeping = %q, want %q", got, c.want)
			}
		})
	}
}

// Linking a new site still takes the newest version composer.json allows.
func TestDetectVersionStillPicksTheNewestForANewSite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	stubRuntimeInstalled(t, false, nil, []string{"8.4", "8.5", "8.6"})
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{"require":{"php":"^8.4"}}`)
	if got := DetectVersionClamped(dir, "", "", "8.4"); got != "8.6" {
		t.Errorf("DetectVersionClamped = %q, want 8.6", got)
	}
}

// Removing the PHP of a paused site must not move the site: resuming it
// installs that version again, unless its files now rule it out.
func TestResumeVersion(t *testing.T) {
	cases := []struct {
		name     string
		composer string
		pin      string
		current  string
		want     string
	}{
		{"an uninstalled version is kept", "^8.2", "", "8.3", "8.3"},
		{"a constraint that rules the version out moves it", "^8.4", "", "8.3", "8.5"},
		{"a pin added while paused wins", "^8.2", "8.4", "8.3", "8.4"},
		{"no recorded version falls back to detection", "^8.2", "", "", "8.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			t.Setenv("HOME", t.TempDir())
			stubRuntimeInstalled(t, false, nil, []string{"8.4", "8.5"})
			dir := t.TempDir()
			writeFile(t, dir, "composer.json", `{"require":{"php":"`+c.composer+`"}}`)
			if c.pin != "" {
				writeFile(t, dir, ".php-version", c.pin+"\n")
			}
			if got := ResumeVersion(dir, c.current); got != c.want {
				t.Errorf("ResumeVersion = %q, want %q", got, c.want)
			}
		})
	}
}
