package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// desktopAppInstalled reports a Lerd desktop app this machine can remove: the
// app bundle on macOS, the user Flatpak on Linux.
var desktopAppInstalled = func() bool { return macDesktopApp() != "" || desktopFlatpakInstalled() }

func removeDesktopApp(deleteData bool) error {
	if app := macDesktopApp(); app != "" {
		return removeMacDesktopApp(app, deleteData)
	}
	if err := removeDesktopFlatpak(deleteData); err != nil {
		return fmt.Errorf("%w; remove it with: flatpak uninstall --user %s", err, desktopFlatpakID)
	}
	return nil
}

// macDesktopAppDirs are where install.sh puts the macOS app.
var macDesktopAppDirs = func() []string {
	home, _ := os.UserHomeDir()
	return []string{"/Applications", filepath.Join(home, "Applications")}
}

// macDesktopAppData is the app's own data, Electron's userData for "Lerd".
var macDesktopAppData = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Lerd")
}

var quitMacDesktopApp = func() {
	_ = exec.Command("osascript", "-e", `tell application id "`+desktopFlatpakID+`" to quit`).Run()
}

// macDesktopApp is the app's bundle, or "" when it is not installed. Only the
// app's own id counts, since lerd's launcher is also called Lerd.app.
func macDesktopApp() string {
	for _, dir := range macDesktopAppDirs() {
		app := filepath.Join(dir, "Lerd.app")
		out, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier",
			filepath.Join(app, "Contents", "Info.plist")).Output()
		if err == nil && strings.TrimSpace(string(out)) == desktopFlatpakID {
			return app
		}
	}
	return ""
}

// removeMacDesktopApp quits the app so it is not left running from a deleted
// bundle, and takes its data only when lerd's data goes too.
func removeMacDesktopApp(app string, deleteData bool) error {
	quitMacDesktopApp()
	if err := os.RemoveAll(app); err != nil {
		return fmt.Errorf("removing %s: %w", app, err)
	}
	if deleteData {
		if err := os.RemoveAll(macDesktopAppData()); err != nil {
			return fmt.Errorf("removing the app's data: %w", err)
		}
	}
	return nil
}
