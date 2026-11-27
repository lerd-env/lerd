package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

// quitMacDesktopApp asks a running app to quit and waits for it to go. Asking
// one that is not running would launch it, AppleScript's tell does that.
var quitMacDesktopApp = func(app string) error {
	running := func() bool {
		return exec.Command("pgrep", "-f", filepath.Join(app, "Contents", "MacOS")+"/").Run() == nil
	}
	if !running() {
		return nil
	}
	_ = exec.Command("osascript", "-e", `tell application id "`+desktopFlatpakID+`" to quit`).Run()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if !running() {
			return nil
		}
	}
	return fmt.Errorf("the Lerd desktop app is still running; quit it and run lerd uninstall again")
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
	if err := quitMacDesktopApp(app); err != nil {
		return err
	}
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
