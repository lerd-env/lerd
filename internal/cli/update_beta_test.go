package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The update notice reads a day-long cache of the latest release on the line
// the install follows, so switching lines has to drop it or the notice keeps
// answering for the old line until tomorrow.
func TestSetBetaChannelForgetsTheCachedLatest(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	if err := os.MkdirAll(filepath.Dir(config.UpdateCheckFile()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.UpdateCheckFile(), []byte(`{"latest_version":"v1.35.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := setBetaChannel(true); err != nil {
		t.Fatalf("setBetaChannel: %v", err)
	}
	if _, err := os.Stat(config.UpdateCheckFile()); !os.IsNotExist(err) {
		t.Errorf("the cached latest release survived the switch, stat err = %v", err)
	}
}
