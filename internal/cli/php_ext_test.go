package cli

import (
	"errors"
	"io"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestValidExtName(t *testing.T) {
	valid := []string{
		"imagick",
		"redis",
		"xdebug",
		"gd",
		"pdo_mysql",
		"soap",
		"APCu",
		"my-ext",
		"ext123",
	}
	for _, name := range valid {
		if !validExtNameRe.MatchString(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}

	invalid := []string{
		"imagick; rm -rf /",
		"ext$(whoami)",
		"ext`id`",
		"ext && echo pwned",
		"ext|cat /etc/passwd",
		"ext\nRUN malicious",
		"ext name",
		"",
		"ext/traversal",
		"ext.so",
	}
	for _, name := range invalid {
		if validExtNameRe.MatchString(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

// A rebuild that fails must not leave the extension declared. Left in, it
// stales every version's image, so the build that cannot succeed is retried by
// every command that follows. php:pkg already reverted; php:ext did not.
func TestPhpExtAdd_revertsTheConfigWhenTheRebuildFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	orig := rebuildFPMImage
	t.Cleanup(func() { rebuildFPMImage = orig })
	rebuildFPMImage = func(string, bool) error { return errors.New("pecl install failed") }

	cmd := newPhpExtAddCmd()
	cmd.SetArgs([]string{"leveldb", "--apk-deps", "leveldb-dev"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil {
		t.Fatal("a failed rebuild must surface an error")
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if slices.Contains(cfg.GetExtensions(), "leveldb") {
		t.Errorf("extension still declared after a failed rebuild: %v", cfg.GetExtensions())
	}
	if deps := cfg.GetExtApkDeps("leveldb"); len(deps) > 0 {
		t.Errorf("apk deps outlived the reverted extension: %v", deps)
	}
}

// Re-adding an already-declared extension must keep it when the rebuild fails:
// the revert would rip out a working declaration this run did not make.
func TestPhpExtAdd_keepsAnAlreadyDeclaredExtensionWhenTheRebuildFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	if err := config.UpdateGlobal(func(c *config.GlobalConfig) { c.AddExtension("leveldb") }); err != nil {
		t.Fatalf("seeding config: %v", err)
	}

	orig := rebuildFPMImage
	t.Cleanup(func() { rebuildFPMImage = orig })
	rebuildFPMImage = func(string, bool) error { return errors.New("pecl install failed") }

	cmd := newPhpExtAddCmd()
	cmd.SetArgs([]string{"leveldb"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil {
		t.Fatal("a failed rebuild must surface an error")
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if !slices.Contains(cfg.GetExtensions(), "leveldb") {
		t.Error("a failed re-add removed the extension that was already declared and working")
	}
}
