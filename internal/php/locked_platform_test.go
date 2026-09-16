package php

import (
	"os"
	"path/filepath"
	"testing"
)

func writePlatformCheck(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	vendor := filepath.Join(dir, "vendor", "composer")
	if err := os.MkdirAll(vendor, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vendor, "platform_check.php"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A project's own composer.json names what the app claims to support; the
// resolved tree underneath it can need more. Laravel 13 declares "php": "^8.3"
// and then pulls Symfony 8 components that every require ">=8.4.1", so composer
// writes a platform_check.php refusing anything below 8.4.1.
//
// Reading only composer.json let `lerd isolate 8.3` be accepted on a fresh
// Laravel 13 site, which then answered 500 on the first request:
//
//	Your Composer dependencies require a PHP version ">= 8.4.1".
func TestLockedPlatformPHPConstraint(t *testing.T) {
	dir := writePlatformCheck(t, `<?php
$issues = array();
if (!(PHP_VERSION_ID >= 80401)) {
    $issues[] = 'Your Composer dependencies require a PHP version ">= 8.4.1".';
}
`)
	if got := LockedPlatformPHPConstraint(dir); got != ">=8.4.1" {
		t.Errorf("LockedPlatformPHPConstraint = %q, want >=8.4.1", got)
	}
}

// Composer pads the patch component to two digits, so 80100 is 8.1.0 and 80401
// is 8.4.1. Getting that wrong would refuse versions that are fine.
func TestLockedPlatformPHPConstraintDecodesVersionIDs(t *testing.T) {
	cases := map[string]string{
		"80000": ">=8.0.0",
		"80100": ">=8.1.0",
		"80209": ">=8.2.9",
		"80401": ">=8.4.1",
		"80510": ">=8.5.10",
	}
	for id, want := range cases {
		dir := writePlatformCheck(t, "<?php\nif (!(PHP_VERSION_ID >= "+id+")) {\n}\n")
		if got := LockedPlatformPHPConstraint(dir); got != want {
			t.Errorf("PHP_VERSION_ID %s -> %q, want %q", id, got, want)
		}
	}
}

// No vendor directory, or a platform check that does not gate on PHP_VERSION_ID
// (composer omits the check when nothing requires it), constrains nothing.
func TestLockedPlatformPHPConstraintAbsent(t *testing.T) {
	if got := LockedPlatformPHPConstraint(t.TempDir()); got != "" {
		t.Errorf("with no vendor dir, got %q, want empty", got)
	}
	dir := writePlatformCheck(t, "<?php\n// nothing to check\n")
	if got := LockedPlatformPHPConstraint(dir); got != "" {
		t.Errorf("with no version gate, got %q, want empty", got)
	}
}

// The locked floor has to actually refuse the version that broke the site, and
// still allow the ones that work.
func TestLockedFloorRefusesTheVersionThatBrokeTheSite(t *testing.T) {
	dir := writePlatformCheck(t, "<?php\nif (!(PHP_VERSION_ID >= 80401)) {\n}\n")
	c := LockedPlatformPHPConstraint(dir)

	if Satisfies("8.3", c) {
		t.Error("8.3 satisfies a >=8.4.1 floor; the isolate that 500s the site would still be allowed")
	}
	for _, v := range []string{"8.4", "8.5"} {
		if !Satisfies(v, c) {
			t.Errorf("%s does not satisfy %s, but it runs the site fine", v, c)
		}
	}
}
