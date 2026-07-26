package node

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// SetDefault must persist into lerd's config only. The test env has no nvm at
// all, so a nil error also proves no `nvm alias default` was attempted.
func TestNvmSetDefaultWritesConfigNotAlias(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("NVM_DIR", tmp)

	m := nvmManager{}
	if err := m.SetDefault("21"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Node.DefaultVersion != "21" {
		t.Errorf("config default_version = %q, want 21", cfg.Node.DefaultVersion)
	}
}

// HasDefault stays a read-only probe of the user's own alias: SetDefault must
// not make it flip, or ensureDefaultNode would skip the eager install.
func TestNvmHasDefaultIgnoresConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("NVM_DIR", tmp)

	m := nvmManager{}
	if err := m.SetDefault("22"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	if m.HasDefault() {
		t.Error("HasDefault should still consult only nvm's alias, not lerd's config")
	}
}
