package node

import (
	"reflect"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestManagerByName(t *testing.T) {
	cases := map[string]string{
		"fnm":   "fnm",
		"nvm":   "nvm",
		"":      "fnm", // unset defaults to fnm
		"bogus": "fnm", // unrecognised falls back to fnm
	}
	for in, want := range cases {
		if got := ManagerByName(in).Name(); got != want {
			t.Errorf("ManagerByName(%q).Name() = %q, want %q", in, got, want)
		}
	}
}

func TestWritesPathShims(t *testing.T) {
	if !WritesPathShims(fnmManager{}) {
		t.Error("fnm should write PATH shims")
	}
	if WritesPathShims(nvmManager{}) {
		t.Error("nvm must not write PATH shims (user's nvm owns node/npm/npx)")
	}
}

func TestManaged_PrefWinsOverMissingShim(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.SetNodeManaged(true)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	if !Managed() {
		t.Fatal("Managed() = false with node.managed=true and no node shim")
	}
	cfg.SetNodeManaged(false)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	if Managed() {
		t.Fatal("Managed() = true with node.managed=false")
	}
}

func TestDedupeMajors(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"full versions to majors", []string{"20.11.0", "20.5.0", "18.0.0"}, []string{"20", "18"}},
		{"strips v prefix", []string{"v22.1.0", "v18.2.0"}, []string{"22", "18"}},
		{"skips non-numeric majors", []string{"lts/iron", "20.0.0"}, []string{"20"}},
		{"empty", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dedupeMajors(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("dedupeMajors(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestUninstallVersions(t *testing.T) {
	full := []string{"20.11.0", "20.5.0", "18.0.0"}

	t.Run("exact version removes only itself", func(t *testing.T) {
		var removed []string
		_ = uninstallVersions("18.0.0", full, func(v string) error {
			removed = append(removed, v)
			return nil
		})
		if !reflect.DeepEqual(removed, []string{"18.0.0"}) {
			t.Errorf("removed = %v, want [18.0.0]", removed)
		}
	})

	t.Run("exact version strips v prefix", func(t *testing.T) {
		var removed []string
		_ = uninstallVersions("v18.0.0", full, func(v string) error {
			removed = append(removed, v)
			return nil
		})
		if !reflect.DeepEqual(removed, []string{"18.0.0"}) {
			t.Errorf("removed = %v, want [18.0.0]", removed)
		}
	})

	t.Run("bare major removes every full version under it", func(t *testing.T) {
		var removed []string
		_ = uninstallVersions("20", full, func(v string) error {
			removed = append(removed, v)
			return nil
		})
		if !reflect.DeepEqual(removed, []string{"20.11.0", "20.5.0"}) {
			t.Errorf("removed = %v, want [20.11.0 20.5.0]", removed)
		}
	})

	t.Run("unmatched major passes through", func(t *testing.T) {
		var removed []string
		_ = uninstallVersions("16", full, func(v string) error {
			removed = append(removed, v)
			return nil
		})
		if !reflect.DeepEqual(removed, []string{"16"}) {
			t.Errorf("removed = %v, want [16] (passthrough)", removed)
		}
	})
}

func TestFnmShellFragments(t *testing.T) {
	// Isolate lerd's data dir so ExecPrefix/ShimScript resolve fnm's path under a
	// temp tree rather than the developer's real install.
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := fnmManager{}
	if prefix := m.ExecPrefix("20"); !strings.Contains(prefix, "exec --using=20 --") {
		t.Errorf("fnm ExecPrefix(20) = %q, want it to contain 'exec --using=20 --'", prefix)
	}
	// Empty version resolves to the default alias.
	if prefix := m.ExecPrefix(""); !strings.Contains(prefix, "--using=default") {
		t.Errorf("fnm ExecPrefix(\"\") = %q, want default alias", prefix)
	}
	shim := m.ShimScript("/home/u/.local/bin/lerd", "npm")
	for _, want := range []string{"#!/bin/sh", "/home/u/.local/bin/lerd", "exec --using=", "-- npm"} {
		if !strings.Contains(shim, want) {
			t.Errorf("fnm ShimScript missing %q:\n%s", want, shim)
		}
	}
}

func TestNvmShellFragments(t *testing.T) {
	m := nvmManager{}
	prefix := m.ExecPrefix("20")
	// The $NVM_BIN guard + PATH prepend is what prevents `exec "$@"` from
	// resolving `node` back to lerd's own shim and fork-bombing, so assert it.
	for _, want := range []string{"nvm.sh", "nvm use", `[ -z "$NVM_BIN" ]`, `PATH="$NVM_BIN:$PATH"`, `exec "$@"`} {
		if !strings.Contains(prefix, want) {
			t.Errorf("nvm ExecPrefix missing %q:\n%s", want, prefix)
		}
	}
	shim := m.ShimScript("/home/u/.local/bin/lerd", "node")
	// nvm is a bash function, so its shims must use bash, not /bin/sh; and the
	// shim must exec node by absolute $NVM_BIN path, never by bare name.
	for _, want := range []string{"#!/usr/bin/env bash", "nvm use", `[ -z "$NVM_BIN" ]`, `exec "$NVM_BIN/node"`} {
		if !strings.Contains(shim, want) {
			t.Errorf("nvm ShimScript missing %q:\n%s", want, shim)
		}
	}
	// Regression guard: the shim must not exec node by bare name (fork-bomb path).
	if strings.Contains(shim, "exec node ") {
		t.Errorf("nvm ShimScript execs node by bare name (fork-bomb risk):\n%s", shim)
	}
}
