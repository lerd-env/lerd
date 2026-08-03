package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// pathShimTestEnv points HOME and the XDG dirs at a temp tree so config,
// BinDir, and the shell rc files all live under the test's control.
func pathShimTestEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, ".local", "share"))
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	return tmp
}

func savePathDisabled(t *testing.T, disabled bool) {
	t.Helper()
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		t.Fatalf("loading config: %v", err)
	}
	cfg.Shims.PathDisabled = disabled
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("saving config: %v", err)
	}
}

func TestAddShellShimsWritesPathEntryByDefault(t *testing.T) {
	tmp := pathShimTestEnv(t)
	t.Setenv("SHELL", "/bin/zsh")

	if err := addShellShims(false); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(filepath.Join(tmp, ".zshrc"))
	if !strings.Contains(string(got), `export PATH="`+config.BinDir()) {
		t.Errorf(".zshrc should contain the lerd PATH export, got:\n%s", got)
	}
}

func TestAddShellShimsSkipsPathEntryWhenDisabled(t *testing.T) {
	tmp := pathShimTestEnv(t)
	t.Setenv("SHELL", "/bin/zsh")
	savePathDisabled(t, true)

	// A prior install already wrote the PATH block; disabling must remove it.
	zshrc := filepath.Join(tmp, ".zshrc")
	os.WriteFile(zshrc, []byte(
		"alias gco='git checkout'\n\n# Lerd\nexport PATH=\""+config.BinDir()+":$PATH\"\n",
	), 0o644)

	if err := addShellShims(false); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(zshrc)
	if strings.Contains(string(got), "export PATH=\""+config.BinDir()) {
		t.Errorf(".zshrc should no longer contain the lerd PATH export, got:\n%s", got)
	}
	if !strings.Contains(string(got), "alias gco") {
		t.Error("pre-existing content should be preserved")
	}
	// The shim scripts themselves must still exist: `lerd php` and internal
	// child processes rely on them regardless of the user's PATH.
	if _, err := os.Stat(filepath.Join(config.BinDir(), "php")); err != nil {
		t.Error("php shim should still be written when the PATH entry is disabled")
	}
}

func TestAddShellShimsKeepsInstallerBinaryPathWhenDisabled(t *testing.T) {
	tmp := pathShimTestEnv(t)
	t.Setenv("SHELL", "/bin/bash")
	savePathDisabled(t, true)

	// install.sh's block puts the lerd binary itself on PATH; it is not the
	// shims entry and must survive path:disable.
	bashrc := filepath.Join(tmp, ".bashrc")
	os.WriteFile(bashrc, []byte(
		"# Added by Lerd installer\nexport PATH=\""+filepath.Join(tmp, ".local", "bin")+":$PATH\"\n"+
			"# Lerd\nexport PATH=\""+config.BinDir()+":$PATH\"\n",
	), 0o644)

	if err := addShellShims(false); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(bashrc)
	if !strings.Contains(string(got), "Added by Lerd installer") {
		t.Error("the installer's lerd-binary PATH block must be kept")
	}
	if strings.Contains(string(got), config.BinDir()) {
		t.Errorf("the shims PATH export should be gone, got:\n%s", got)
	}
}

func TestAddShellShimsRemovesFishEntryWhenDisabled(t *testing.T) {
	tmp := pathShimTestEnv(t)
	t.Setenv("SHELL", "/usr/bin/fish")
	savePathDisabled(t, true)

	fishConf := filepath.Join(tmp, ".config", "fish", "conf.d", "lerd.fish")
	os.MkdirAll(filepath.Dir(fishConf), 0o755)
	os.WriteFile(fishConf, []byte("set -gx PATH "+config.BinDir()+" $PATH\n"), 0o644)

	if err := addShellShims(false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(fishConf); !os.IsNotExist(err) {
		got, _ := os.ReadFile(fishConf)
		t.Errorf("lerd.fish should be removed when the PATH entry is disabled, got:\n%s", got)
	}
}

func TestApplyPathShimRoundTrip(t *testing.T) {
	tmp := pathShimTestEnv(t)
	t.Setenv("SHELL", "/bin/zsh")
	zshrc := filepath.Join(tmp, ".zshrc")
	os.WriteFile(zshrc, []byte("# Lerd\nexport PATH=\""+config.BinDir()+":$PATH\"\n"), 0o644)

	if err := applyPathShim(true); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.LoadGlobal()
	if cfg == nil || !cfg.Shims.PathDisabled {
		t.Error("path:disable should persist shims.path_disabled")
	}
	got, _ := os.ReadFile(zshrc)
	if strings.Contains(string(got), config.BinDir()) {
		t.Errorf("path:disable should remove the PATH export, got:\n%s", got)
	}

	if err := applyPathShim(false); err != nil {
		t.Fatal(err)
	}
	cfg, _ = config.LoadGlobal()
	if cfg == nil || cfg.Shims.PathDisabled {
		t.Error("path:enable should persist shims.path_disabled=false")
	}
	got, _ = os.ReadFile(zshrc)
	if !strings.Contains(string(got), `export PATH="`+config.BinDir()) {
		t.Errorf("path:enable should write the PATH export back, got:\n%s", got)
	}
}
