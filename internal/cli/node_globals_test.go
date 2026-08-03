package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestUserNPMPrefixFromEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("npm_config_prefix", "/opt/npm-globals")
	t.Setenv("NPM_CONFIG_PREFIX", "")
	if got := userNPMPrefix(); got != "/opt/npm-globals" {
		t.Errorf("userNPMPrefix() = %q, want /opt/npm-globals", got)
	}

	t.Setenv("npm_config_prefix", "")
	t.Setenv("NPM_CONFIG_PREFIX", "/opt/upper")
	if got := userNPMPrefix(); got != "/opt/upper" {
		t.Errorf("userNPMPrefix() = %q, want /opt/upper", got)
	}
}

func TestUserNPMPrefixFromNpmrc(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("npm_config_prefix", "")
	t.Setenv("NPM_CONFIG_PREFIX", "")

	os.WriteFile(filepath.Join(tmp, ".npmrc"), []byte(
		"; comment\nregistry=https://registry.npmjs.org/\nprefix = ~/.npm-global\n",
	), 0o644)

	if got, want := userNPMPrefix(), filepath.Join(tmp, ".npm-global"); got != want {
		t.Errorf("userNPMPrefix() = %q, want %q", got, want)
	}
}

func TestUserNPMPrefixEmptyWhenUnset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("npm_config_prefix", "")
	t.Setenv("NPM_CONFIG_PREFIX", "")
	if got := userNPMPrefix(); got != "" {
		t.Errorf("userNPMPrefix() = %q, want empty", got)
	}
}

func TestNpmGlobalPrefixEnvRespectsUserPrefix(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, ".local", "share"))
	t.Setenv("npm_config_prefix", "/opt/npm-globals")
	t.Setenv("NPM_CONFIG_PREFIX", "")

	env, lerdOwned := npmGlobalPrefixEnv()
	if lerdOwned {
		t.Error("a user-configured prefix must not be lerd-owned (no wrapper sync)")
	}
	if !reflect.DeepEqual(env, []string{"npm_config_prefix=/opt/npm-globals"}) {
		t.Errorf("env = %v, want the user's own prefix", env)
	}
}

func TestNpmGlobalPrefixEnvDefaultsToLerdPrefix(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, ".local", "share"))
	t.Setenv("npm_config_prefix", "")
	t.Setenv("NPM_CONFIG_PREFIX", "")

	env, lerdOwned := npmGlobalPrefixEnv()
	if !lerdOwned {
		t.Error("without a user prefix the lerd prefix is used and wrappers sync")
	}
	want := []string{"npm_config_prefix=" + config.NodeGlobalDir()}
	if !reflect.DeepEqual(env, want) {
		t.Errorf("env = %v, want %v", env, want)
	}
	if _, err := os.Stat(filepath.Join(config.NodeGlobalDir(), "bin")); err != nil {
		t.Error("the lerd prefix bin dir should be created")
	}
}

func TestNodeGlobalPackages(t *testing.T) {
	prefix := t.TempDir()
	mods := filepath.Join(prefix, "lib", "node_modules")
	writePkg := func(dir, name, version string) {
		full := filepath.Join(mods, dir)
		os.MkdirAll(full, 0o755)
		os.WriteFile(filepath.Join(full, "package.json"),
			[]byte(`{"name":"`+name+`","version":"`+version+`"}`), 0o644)
	}
	writePkg("codex", "codex", "1.2.3")
	writePkg("@scope/tool", "@scope/tool", "0.1.0")
	os.MkdirAll(filepath.Join(mods, ".bin"), 0o755)
	os.MkdirAll(filepath.Join(mods, "noversion"), 0o755)

	got := nodeGlobalPackages(prefix)
	want := []string{"@scope/tool@0.1.0", "codex@1.2.3", "noversion"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nodeGlobalPackages() = %v, want %v", got, want)
	}
}

func TestNodeGlobalPackagesEmptyPrefix(t *testing.T) {
	if got := nodeGlobalPackages(filepath.Join(t.TempDir(), "missing")); got != nil {
		t.Errorf("nodeGlobalPackages() on a missing prefix = %v, want nil", got)
	}
}

func TestRemoveNodeGlobalWrappers(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "codex"),
		[]byte("#!/bin/sh\n# "+nodeShimMarker+"\nexec fnm exec --using default codex\n"), 0o755)
	os.WriteFile(filepath.Join(bin, "php"),
		[]byte("#!/bin/sh\nexec lerd php \"$@\"\n"), 0o755)
	fakeBinary := append([]byte{0x7f, 'E', 'L', 'F', 0, 0, 0, 0}, []byte(nodeShimMarker)...)
	os.WriteFile(filepath.Join(bin, "native"), fakeBinary, 0o755)

	removeNodeGlobalWrappers(bin)

	if _, err := os.Stat(filepath.Join(bin, "codex")); !os.IsNotExist(err) {
		t.Error("marked npm-global wrapper should be removed")
	}
	if _, err := os.Stat(filepath.Join(bin, "php")); err != nil {
		t.Error("unmarked shim must be left alone")
	}
	if _, err := os.Stat(filepath.Join(bin, "native")); err != nil {
		t.Error("native binaries must be left alone even if they contain the marker bytes")
	}
}

func TestFormatNodeGlobalsNote(t *testing.T) {
	got := formatNodeGlobalsNote([]string{"codex@1.2.3", "@scope/tool@0.1.0"})
	if !strings.Contains(got, "codex@1.2.3") || !strings.Contains(got, "@scope/tool@0.1.0") {
		t.Errorf("note should list every package, got %q", got)
	}
}
