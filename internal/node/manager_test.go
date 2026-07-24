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
	// Honouring `nvm use` exit status catches a missing pin; $NVM_BIN alone is
	// already set by sourcing nvm.sh and is not enough.
	for _, want := range []string{"nvm.sh", "nvm use", `PATH="$NVM_BIN:$PATH"`, `exec "$@"`} {
		if !strings.Contains(prefix, want) {
			t.Errorf("nvm ExecPrefix missing %q:\n%s", want, prefix)
		}
	}
	if strings.Contains(prefix, `[ -z "$NVM_BIN" ]`) {
		t.Errorf("nvm ExecPrefix still guards on empty NVM_BIN (false negative):\n%s", prefix)
	}
	shim := m.ShimScript("/unused", "node")
	if !strings.Contains(shim, "does not install PATH shims") {
		t.Errorf("nvm ShimScript should be a stub explaining shims are unused:\n%s", shim)
	}
}

func TestNvmApplyEnv_ExportsAfterActivation(t *testing.T) {
	m := nvmManager{}
	cmd := m.Command("20", "npm", []string{"root", "-g"})
	m.ApplyEnv(cmd, []string{"npm_config_prefix=/tmp/lerd-global"})
	script := ""
	for i := 0; i+1 < len(cmd.Args); i++ {
		if cmd.Args[i] == "-c" {
			script = cmd.Args[i+1]
			break
		}
	}
	if script == "" {
		t.Fatal("no -c script on command")
	}
	useIdx := strings.Index(script, "nvm use")
	exportIdx := strings.Index(script, "export npm_config_prefix=")
	execIdx := strings.Index(script, `exec "$@"`)
	if useIdx < 0 || exportIdx < 0 || execIdx < 0 {
		t.Fatalf("script missing pieces:\n%s", script)
	}
	if !(useIdx < exportIdx && exportIdx < execIdx) {
		t.Errorf("export must sit after nvm use and before exec:\n%s", script)
	}
	if strings.Contains(script, "export npm_config_prefix=/tmp/lerd-global") {
		t.Errorf("value must be shell-quoted:\n%s", script)
	}
	if !strings.Contains(script, "export npm_config_prefix='/tmp/lerd-global'") {
		t.Errorf("expected quoted export:\n%s", script)
	}
}

func TestFnmApplyEnv_AppendsToCmdEnv(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := fnmManager{}
	cmd := m.Command("20", "npm", []string{"root", "-g"})
	cmd.Env = []string{"PATH=/bin"}
	m.ApplyEnv(cmd, []string{"npm_config_prefix=/tmp/g"})
	found := false
	for _, e := range cmd.Env {
		if e == "npm_config_prefix=/tmp/g" {
			found = true
		}
	}
	if !found {
		t.Errorf("fnm ApplyEnv did not append prefix: %v", cmd.Env)
	}
}
