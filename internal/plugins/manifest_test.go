package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePlugin lays out a plugin dir named name with the given manifest and an
// executable bin/run, which is what most manifests below point at.
func writePlugin(t *testing.T, root, name, manifest string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "run"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReadManifest_SkeletonIsValid(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	dir := t.TempDir()
	if err := Scaffold("hello", filepath.Join(dir, "hello")); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	p, err := ReadPlugin(filepath.Join(dir, "hello"))
	if err != nil {
		t.Fatalf("the shipped boilerplate must validate: %v", err)
	}
	if len(p.Commands) == 0 || len(p.Hooks) == 0 || p.MCP == nil || len(p.API) == 0 || p.UI == nil || p.UI.Page == nil || p.UI.SiteTab == nil {
		t.Errorf("the boilerplate should exercise every surface, got %+v", p.Manifest)
	}
	if p.Docs == "" {
		t.Error("the boilerplate should ship docs, so every new plugin starts with some")
	}
	if got := strings.Join(p.Requirements(), ","); got != "/bin/sh,curl" {
		t.Errorf("the boilerplate requires %q, want sh and the curl bin/sites calls", got)
	}
}

func TestReadManifest_Refusals(t *testing.T) {
	cases := []struct {
		name, dir, manifest, want string
	}{
		{"newer api version", "p", "api_version: 2\nname: p\n", "api_version 2"},
		{"missing api version", "p", "name: p\n", "api_version"},
		{"name differs from dir", "p", "api_version: 1\nname: q\n", `"q"`},
		{"bad name", "P_x", "api_version: 1\nname: P_x\n", "P_x"},
		{"unknown event", "p", "api_version: 1\nname: p\nhooks:\n  site.exploded: bin/run\n", "site.exploded"},
		{"run escapes dir", "p", "api_version: 1\nname: p\ncommands:\n  - {name: x, run: ../../bin/sh}\n", "../../bin/sh"},
		{"absolute run", "p", "api_version: 1\nname: p\ncommands:\n  - {name: x, run: /bin/sh}\n", "/bin/sh"},
		{"missing run file", "p", "api_version: 1\nname: p\ncommands:\n  - {name: x, run: bin/nope}\n", "bin/nope"},
		{"duplicate command", "p", "api_version: 1\nname: p\ncommands:\n  - {name: x, run: bin/run}\n  - {name: x, run: bin/run}\n", `"x"`},
		{"unknown method", "p", "api_version: 1\nname: p\napi:\n  - {path: x, methods: [BREW], run: bin/run}\n", "BREW"},
		{"api path escapes", "p", "api_version: 1\nname: p\napi:\n  - {path: ../x, run: bin/run}\n", "../x"},
		{"ui entry outside ui/", "p", "api_version: 1\nname: p\nui:\n  page: {title: P, entry: bin/run}\n", "bin/run"},
		{"overview card outside ui/", "p", "api_version: 1\nname: p\nui:\n  overview_card: {title: P, entry: bin/run}\n", "bin/run"},
		{"overview card without a title", "p", "api_version: 1\nname: p\nui:\n  overview_card: {entry: ui/card.html}\n", "title"},
		{"page script outside ui/", "p", "api_version: 1\nname: p\nui:\n  page_script: bin/run\n", "bin/run"},
		{"expected exit 0", "p", "api_version: 1\nname: p\ncommands:\n  - {name: x, run: bin/run, expected_exits: [0]}\n", "expected exit 0"},
		{"expected exit past 255", "p", "api_version: 1\nname: p\nsite_commands:\n  - {name: x, label: X, run: bin/run, expected_exits: [1, 256]}\n", "expected exit 256"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writePlugin(t, t.TempDir(), c.dir, c.manifest)
			_, err := ReadPlugin(dir)
			if err == nil {
				t.Fatal("expected the manifest to be refused")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error should name %q, got %v", c.want, err)
			}
		})
	}
}

func TestReadManifest_RunMustBeExecutable(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\ncommands:\n  - {name: x, run: bin/run}\n")
	if err := os.Chmod(filepath.Join(dir, "bin", "run"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPlugin(dir); err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("a non-executable run file should be refused, got %v", err)
	}
}

// A newer lerd may add keys; a plugin written for it should still load here as
// long as it keeps to the api_version this binary knows.
func TestReadManifest_IgnoresUnknownKeys(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\nsomething_new: true\n")
	if _, err := ReadPlugin(dir); err != nil {
		t.Fatalf("unknown keys should be ignored: %v", err)
	}
}

func TestReadManifest_ExpectedExits(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\ncommands:\n  - {name: x, run: bin/run, expected_exits: [1]}\nsite_commands:\n  - {name: check, label: Check, run: bin/run, expected_exits: [101]}\n")
	p, err := ReadPlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Commands[0].Expects(1) || p.Commands[0].Expects(2) {
		t.Errorf("command expects %v", p.Commands[0].ExpectedExits)
	}
	if !p.SiteCommands[0].Expects(101) || p.SiteCommands[0].Expects(1) {
		t.Errorf("site command expects %v", p.SiteCommands[0].ExpectedExits)
	}
}
