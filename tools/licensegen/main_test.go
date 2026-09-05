package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectLicense(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"mit", "MIT License\n\nPermission is hereby granted, free of charge, to any person", "MIT"},
		{"isc", "ISC License\n\nPermission to use, copy, modify, and/or distribute this software", "ISC"},
		{"apache", "Apache License\nVersion 2.0, January 2004", "Apache-2.0"},
		{"mpl", "Mozilla Public License Version 2.0", "MPL-2.0"},
		{"gpl", "GNU GENERAL PUBLIC LICENSE\nVersion 3", "GPL"},
		{"lgpl", "GNU LESSER GENERAL PUBLIC LICENSE", "LGPL"},
		{"bsd2", "Redistribution and use in source and binary forms, with or without modification", "BSD-2-Clause"},
		{"bsd3", "Redistribution and use in source and binary forms\nNeither the name of the copyright holder", "BSD-3-Clause"},
		{"unlicense", "This is free and unencumbered software released into the public domain.", "Unlicense"},
		{"empty", "", "Unknown"},
		{"unrecognised", "Do what you want, I guess.", "Unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detectLicense(c.text); got != c.want {
				t.Errorf("detectLicense(%q) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// The GNU family shares wording with the permissive texts that quote it, so
// the ordering inside detectLicense is load bearing.
func TestDetectLicenseLGPLIsNotReportedAsGPL(t *testing.T) {
	text := "GNU LESSER GENERAL PUBLIC LICENSE\n\nThis version of the GNU Lesser General Public License incorporates the terms of the GNU General Public License."
	if got := detectLicense(text); got != "LGPL" {
		t.Errorf("detectLicense = %q, want LGPL", got)
	}
}

func TestLicenseTextConcatenatesEveryNoticeFile(t *testing.T) {
	got, err := licenseText(filepath.Join("testdata", "apachemod"))
	if err != nil {
		t.Fatalf("licenseText: %v", err)
	}
	if !strings.Contains(got, "Apache License") {
		t.Errorf("license body missing from %q", got)
	}
	if !strings.Contains(got, "Example Corp") {
		t.Errorf("NOTICE body missing from %q", got)
	}
}

func TestLicenseTextIgnoresUnrelatedFiles(t *testing.T) {
	got, err := licenseText(filepath.Join("testdata", "node_modules", "plain"))
	if err != nil {
		t.Fatalf("licenseText: %v", err)
	}
	if strings.Contains(got, "readme") {
		t.Errorf("readme was picked up as a license: %q", got)
	}
}

func TestLicenseTextIsEmptyWhenNothingIsShipped(t *testing.T) {
	got, err := licenseText(filepath.Join("testdata", "node_modules", "bare"))
	if err != nil {
		t.Fatalf("licenseText: %v", err)
	}
	if got != "" {
		t.Errorf("licenseText = %q, want empty", got)
	}
}

func TestDeclaredLicense(t *testing.T) {
	if got := declaredLicense(json.RawMessage(`"MIT"`)); got != "MIT" {
		t.Errorf("string form = %q, want MIT", got)
	}
	if got := declaredLicense(json.RawMessage(`{"type":"BSD-3-Clause","url":"x"}`)); got != "BSD-3-Clause" {
		t.Errorf("object form = %q, want BSD-3-Clause", got)
	}
	if got := declaredLicense(nil); got != "" {
		t.Errorf("missing form = %q, want empty", got)
	}
}

func TestNpmPackages(t *testing.T) {
	pkgs, err := npmPackages(filepath.Join("testdata", "node_modules"))
	if err != nil {
		t.Fatalf("npmPackages: %v", err)
	}
	got := map[string]pkg{}
	for _, p := range pkgs {
		got[p.Name] = p
	}
	if len(got) != 3 {
		t.Fatalf("found %d packages, want 3: %v", len(got), got)
	}
	if got["plain"].License != "MIT" || !strings.Contains(got["plain"].Text, "Plain Author") {
		t.Errorf("plain = %+v", got["plain"])
	}
	// A nested copy under a parent's own node_modules still has to be disclosed.
	if got["nested"].Version != "2.0.0" {
		t.Errorf("nested package was not walked: %+v", got["nested"])
	}
	// No license field and no license file: the summary must say so rather
	// than quietly claim a license the package never granted.
	if got["bare"].License != "Unknown" {
		t.Errorf("bare = %+v, want Unknown", got["bare"])
	}
}

func TestNpmPackagesFailsLoudlyWithoutNodeModules(t *testing.T) {
	if _, err := npmPackages(filepath.Join("testdata", "does-not-exist")); err == nil {
		t.Fatal("expected an error when node_modules is missing")
	}
}

func TestNpmPackagesAreSortedForAStableDiff(t *testing.T) {
	pkgs, err := npmPackages(filepath.Join("testdata", "node_modules"))
	if err != nil {
		t.Fatalf("npmPackages: %v", err)
	}
	for i := 1; i < len(pkgs); i++ {
		if pkgs[i-1].Name > pkgs[i].Name {
			t.Fatalf("packages out of order at %d: %s before %s", i, pkgs[i-1].Name, pkgs[i].Name)
		}
	}
}

func TestRenderGroupsIdenticalTexts(t *testing.T) {
	shared := "BSD text\nRedistribution and use in source and binary forms\n"
	pkgs := []pkg{
		{Name: "golang.org/x/net", Version: "v1", License: "BSD-2-Clause", Text: shared},
		{Name: "golang.org/x/sys", Version: "v2", License: "BSD-2-Clause", Text: shared},
		{Name: "solo", Version: "v3", License: "MIT", Text: "MIT text\n"},
	}
	out := render(pkgs, nil)
	if n := strings.Count(out, "BSD text"); n != 1 {
		t.Errorf("shared text reproduced %d times, want 1", n)
	}
	if !strings.Contains(out, "### golang.org/x/net v1, golang.org/x/sys v2") {
		t.Errorf("grouped heading missing from:\n%s", out)
	}
	if !strings.Contains(out, "| solo | v3 | MIT |") {
		t.Errorf("summary row missing from:\n%s", out)
	}
	if !strings.Contains(out, "None.") {
		t.Errorf("empty section should say so:\n%s", out)
	}
}

func TestRenderNamesPackagesWithoutALicenseFile(t *testing.T) {
	out := render([]pkg{{Name: "mystery", Version: "v1", License: "Unknown"}}, nil)
	if !strings.Contains(out, "No license file is distributed with this package.") {
		t.Errorf("missing-license note absent from:\n%s", out)
	}
}
