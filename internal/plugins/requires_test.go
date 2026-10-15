package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func script(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestInterpreter(t *testing.T) {
	cases := map[string]string{
		"#!/bin/sh\necho":                       "/bin/sh",
		"#!/usr/bin/env python3\nprint()":       "python3",
		"#! /usr/bin/env -S node --no-warnings": "node",
		"#!/usr/bin/env FOO=1 php\n":            "php",
		"\x7fELF binary":                        "",
		"echo no shebang":                       "",
	}
	for body, want := range cases {
		dir := t.TempDir()
		script(t, dir, "run", body)
		if got := interpreter(filepath.Join(dir, "run")); got != want {
			t.Errorf("%q: interpreter %q, want %q", body, got, want)
		}
	}
}

func TestRequirements_CombineInterpretersAndDeclaredCommands(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\nrequires: [jq]\ncommands:\n  - {name: a, run: bin/a}\n  - {name: b, run: bin/b}\nhooks:\n  site.linked: bin/run\n")
	script(t, dir, "bin/a", "#!/usr/bin/env sh\n")
	script(t, dir, "bin/b", "#!/usr/bin/env sh\n")
	p, err := ReadPlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.Requirements(), ","); got != "/bin/sh,jq,sh" {
		t.Errorf("requirements %q", got)
	}
}

// A plugin whose interpreter is missing is refused by name, not left to fail
// the first time someone runs it.
func TestReadPlugin_RefusesWhatThisMachineLacks(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\ncommands:\n  - {name: a, run: bin/run}\n")
	script(t, dir, "bin/run", "#!/usr/bin/env lerd-test-no-such-interpreter\n")
	_, err := ReadPlugin(dir)
	if err == nil || !strings.Contains(err.Error(), "lerd-test-no-such-interpreter") {
		t.Fatalf("err = %v", err)
	}
	dir = writePlugin(t, t.TempDir(), "q", "api_version: 1\nname: q\nrequires: [lerd-test-no-such-tool]\n")
	if _, err := ReadPlugin(dir); err == nil || !strings.Contains(err.Error(), "lerd-test-no-such-tool") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadPlugin_RefusesARequirementThatIsAPath(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\nrequires: [../bin/x]\n")
	if _, err := ReadPlugin(dir); err == nil || !strings.Contains(err.Error(), "../bin/x") {
		t.Fatalf("err = %v", err)
	}
}

// lerd's own php and node shims count as installed even when the service a
// plugin runs under was started with a PATH that lacks lerd's bin folder.
func TestMissing_LooksInLerdsOwnBinFolder(t *testing.T) {
	isolate(t)
	t.Setenv("PATH", "/usr/bin:/bin")
	script(t, filepath.Join(os.Getenv("XDG_DATA_HOME"), "lerd"), "bin/lerd-test-shim", "#!/bin/sh\n")
	if got := Missing([]string{"lerd-test-shim", "sh"}); len(got) != 0 {
		t.Errorf("missing %v, want none", got)
	}
}

// A plugin runs on this machine, and lerd's php runs in a container, so a PHP
// script would run without the plugin's context.
func TestReadPlugin_RefusesAPHPScript(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\ncommands:\n  - {name: a, run: bin/run}\n")
	script(t, dir, "bin/run", "#!/usr/bin/env php\n<?php echo 1;\n")
	if _, err := ReadPlugin(dir); err == nil || !strings.Contains(err.Error(), "bin/run") || !strings.Contains(err.Error(), "PHP") {
		t.Fatalf("err = %v", err)
	}
}
