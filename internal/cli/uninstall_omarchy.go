package cli

import (
	"os"
	"os/exec"
	"path/filepath"
)

// glancePluginID is the Omarchy bar plugin install.sh adds in place of the tray.
const glancePluginID = "sh.lerd.glance"

// runGlanceRemove runs the Omarchy plugin command, indirected for tests.
var runGlanceRemove = func(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	// Omarchy's plugin commands refuse to run without OMARCHY_PATH, which a
	// shell outside the Omarchy session may not carry.
	if os.Getenv("OMARCHY_PATH") == "" {
		cmd.Env = append(os.Environ(), "OMARCHY_PATH=/usr/share/omarchy")
	}
	return cmd.Run()
}

// removeOmarchyGlance takes the Glance plugin off the Omarchy bar, as
// install.sh --uninstall does. It reports whether there was one to remove.
func removeOmarchyGlance() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(home, ".config/omarchy/plugins", glancePluginID)); err != nil {
		return false, nil
	}
	return true, runGlanceRemove("omarchy-plugin-remove", glancePluginID, "--yes")
}
