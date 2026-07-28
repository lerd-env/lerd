package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// setDefaultPHP persists a global config whose default PHP version is v.
func setDefaultPHP(t *testing.T, v string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	// macOS reads the installed PHP set from ~/Library/LaunchAgents, so without
	// a temp home the developer's own versions decide what is "installed".
	t.Setenv("HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.DNS.TLD = "test"
	cfg.PHP.DefaultVersion = v
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
}

func fwWithRange(min, max string) *config.Framework {
	return &config.Framework{Name: "fw", Label: "FW", PHP: config.FrameworkPHP{Min: min, Max: max}}
}

// A framework whose supported max is below the machine default must scaffold
// under a version inside its range, not the default — the core of issue #1165.
func TestScaffoldPHPVersion_clampsDefaultAboveTheRange(t *testing.T) {
	setDefaultPHP(t, "8.5")

	// No in-range version is installed in the temp home, so the clamp falls back
	// to the framework minimum, which is still inside the supported range.
	got := scaffoldPHPVersion(fwWithRange("8.1", "8.3"))
	if got != "8.1" {
		t.Errorf("scaffoldPHPVersion = %q, want 8.1 (clamped into 8.1-8.3, not the 8.5 default)", got)
	}
}

// A default already inside the framework's range is left as-is.
func TestScaffoldPHPVersion_keepsAnInRangeDefault(t *testing.T) {
	setDefaultPHP(t, "8.3")

	if got := scaffoldPHPVersion(fwWithRange("8.1", "8.4")); got != "8.3" {
		t.Errorf("scaffoldPHPVersion = %q, want the in-range default 8.3", got)
	}
}

// A framework that declares no range leaves the scaffold on the default, so the
// version stays empty and the caller runs the existing default path.
func TestScaffoldPHPVersion_emptyWhenNoRange(t *testing.T) {
	setDefaultPHP(t, "8.5")

	if got := scaffoldPHPVersion(fwWithRange("", "")); got != "" {
		t.Errorf("scaffoldPHPVersion = %q, want empty when the framework declares no range", got)
	}
}
