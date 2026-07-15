package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestRunTrayIconSet_PersistsHighContrast(t *testing.T) {
	withTempXDG(t)

	if err := runTrayIconSet(true); err != nil {
		t.Fatalf("runTrayIconSet(true): %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsHighContrastTrayIcon() {
		t.Error("high-contrast icon preference not persisted")
	}
}

func TestRunTrayIconSet_DefaultRoundTrip(t *testing.T) {
	withTempXDG(t)

	_ = runTrayIconSet(true)
	if err := runTrayIconSet(false); err != nil {
		t.Fatalf("runTrayIconSet(false): %v", err)
	}
	cfg, _ := config.LoadGlobal()
	if cfg.IsHighContrastTrayIcon() {
		t.Error("preference should be back to default after setting default")
	}
}

func TestNewTrayCmd_HasIconSubcommand(t *testing.T) {
	cmd := NewTrayCmd()
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "icon" {
			found = true
		}
	}
	if !found {
		t.Error("tray command is missing the icon subcommand")
	}
}
