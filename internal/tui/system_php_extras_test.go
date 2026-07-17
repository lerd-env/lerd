package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// installPHPVersion makes a version look installed to phpPkg.ListInstalled,
// which reads the quadlet dir, so systemRows renders a row for it.
func installPHPVersion(t *testing.T, version string) {
	t.Helper()
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	short := strings.ReplaceAll(version, ".", "")
	unit := filepath.Join(dir, "lerd-php"+short+"-fpm.container")
	if err := os.WriteFile(unit, []byte("[Container]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func systemRowLabels(m *Model) map[string]string {
	out := map[string]string{}
	for _, r := range m.systemRows() {
		out[r.label] = r.value
	}
	return out
}

// The Extras row must reach the rendered System view, not just the helper: this
// drives systemRows itself so the wiring is covered.
func TestSystemRows_showsWhatAVersionsImageCarries(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_RUNTIME_DIR", tmp)
	installPHPVersion(t, "8.4")

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AddPackage("chromium")
	cfg.SetRealised("8.4", config.RealisedPHPSet{Packages: []string{"chromium"}})
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}

	m := &Model{}
	if got := systemRowLabels(m)["Extras · PHP 8.4"]; got != "chromium" {
		t.Errorf("Extras row = %q, want chromium", got)
	}
}

// Dashboard clutter is a hard line: a user who declares nothing must not get an
// extra row per PHP version telling them so.
func TestSystemRows_noExtrasRowWhenNothingDeclared(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_RUNTIME_DIR", tmp)
	installPHPVersion(t, "8.4")

	m := &Model{}
	for label := range systemRowLabels(m) {
		if strings.HasPrefix(label, "Extras · PHP") {
			t.Errorf("an Extras row appeared with nothing declared: %q", label)
		}
	}
}
