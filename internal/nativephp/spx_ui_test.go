package nativephp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// SPX serves its control panel from files, not from the extension, and the
// path it is compiled with only exists inside the image. Without this the
// profiler answered "File not found." for its own dashboard.
func TestOverridePointsSPXAtItsWebUI(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.SpxWebUIDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SpxWebUIDir(), "index.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := overrideIni("8.4"); !strings.Contains(got, "spx.http_ui_assets_dir="+config.SpxWebUIDir()) {
		t.Errorf("override should point SPX at the installed web UI:\n%s", got)
	}
}

// Naming a directory that is not there would leave SPX serving nothing from an
// empty path rather than falling back to whatever it was built with.
func TestOverrideOmitsSPXWebUIWhenAbsent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if got := overrideIni("8.4"); strings.Contains(got, "spx.http_ui_assets_dir") {
		t.Errorf("no web UI installed, so nothing should be pointed at:\n%s", got)
	}
}
