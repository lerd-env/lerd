package desktopapp

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/desktopnotify"
)

//go:embed assets/lerd.icns
var iconICNS []byte

//go:embed assets/lerd-mark.png
var markPNG []byte

//go:embed assets/launcher.applescript
var launcherScript string

// bundleID is the launcher's own identifier, distinct from the lerd-desktop
// app's, so installing both does not make them fight over LaunchServices.
const bundleID = "sh.lerd.launcher"

// Tools macOS ships. osacompile is what turns the launcher into an application
// that can draw a native progress window; the rest are held in variables so a
// test can point them somewhere harmless.
var (
	osacompilePath = "/usr/bin/osacompile"
	plistBuddyPath = "/usr/libexec/PlistBuddy"
	codesignPath   = "/usr/bin/codesign"
	lsregisterPath = "/System/Library/Frameworks/CoreServices.framework/" +
		"Frameworks/LaunchServices.framework/Support/lsregister"
)

// desktopAppInstalled reports whether the Lerd desktop app is there. It starts
// lerd and opens the dashboard itself, so the launcher would be a second icon
// named Lerd doing the same job.
var desktopAppInstalled = desktopnotify.AppInstalled

// appsDir is the user's application folder. ~/Applications is deliberate:
// /Applications needs root, and lerd never asks for it.
var appsDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Applications")
}

// Path is where the launcher lives, empty when the home directory is unknown.
func Path() string {
	dir := appsDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, Name+".app")
}

// Install builds the launcher bundle and returns its path. Rebuilt on every run
// so an upgraded lerd repoints it at the binary that is live now. With the
// desktop app installed it removes the launcher instead and returns "".
//
// The bundle is compiled rather than hand-written: an AppleScript application
// can draw the progress window that makes clicking the icon readable while the
// start runs, which a shell script in a bundle cannot.
func Install() (string, error) {
	app := Path()
	if app == "" {
		return "", fmt.Errorf("locating the home directory")
	}
	if desktopAppInstalled() {
		return "", Remove()
	}
	if !replaceable(app) {
		return "", fmt.Errorf("%s is another application, not lerd's launcher; leaving it alone", app)
	}
	if _, err := os.Stat(osacompilePath); err != nil {
		return "", fmt.Errorf("osacompile is not available: %w", err)
	}

	work, err := os.MkdirTemp("", "lerd-launcher")
	if err != nil {
		return "", fmt.Errorf("creating a build directory: %w", err)
	}
	defer os.RemoveAll(work)

	src := filepath.Join(work, "launcher.applescript")
	script := strings.ReplaceAll(launcherScript, "{{LERD_BIN}}", config.LerdBinary())
	if err := os.WriteFile(src, []byte(script), 0644); err != nil {
		return "", fmt.Errorf("writing the launcher script: %w", err)
	}

	built := filepath.Join(work, Name+".app")
	if out, err := exec.Command(osacompilePath, "-o", built, src).CombinedOutput(); err != nil {
		return "", fmt.Errorf("compiling the launcher: %w\n%s", err, out)
	}
	if err := dressBundle(built); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(app), 0755); err != nil {
		return "", fmt.Errorf("creating %s: %w", filepath.Dir(app), err)
	}
	// Replace rather than merge: a rebuilt bundle carries a different set of
	// compiled resources, and leftovers from the previous one are what make an
	// upgraded applet fail to launch.
	if err := os.RemoveAll(app); err != nil {
		return "", fmt.Errorf("removing the previous launcher: %w", err)
	}
	if err := os.Rename(built, app); err != nil {
		return "", fmt.Errorf("installing the launcher: %w", err)
	}

	// LaunchServices indexes a bundle off its modification time, so a rewritten
	// bundle that keeps the old timestamp keeps the old identity in Spotlight.
	now := time.Now()
	_ = os.Chtimes(app, now, now)
	registerBundle(app)
	return app, nil
}

// dressBundle gives the compiled applet lerd's icon and name. osacompile writes
// a generic script-application identity, which would otherwise show up in
// Launchpad, Spotlight and the Dock as a nameless applet with the stock icon.
func dressBundle(app string) error {
	res := filepath.Join(app, "Contents", "Resources")
	if err := os.WriteFile(filepath.Join(res, "applet.icns"), iconICNS, 0644); err != nil {
		return fmt.Errorf("writing the icon: %w", err)
	}
	// The splash draws from a PNG rather than the icns: NSImage picks a
	// representation out of an icns and mattes it to white at the size the
	// window uses, which put a white box behind the mark.
	if err := os.WriteFile(filepath.Join(res, "lerd-mark.png"), markPNG, 0644); err != nil {
		return fmt.Errorf("writing the splash mark: %w", err)
	}
	// Two things outrank the icns file and both have to go, or the icon written
	// above is never the one drawn: CFBundleIconName points at the compiled
	// asset catalog, which macOS prefers, and the catalog still holds
	// osacompile's stock applet artwork.
	if err := os.Remove(filepath.Join(res, "Assets.car")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing the stock asset catalog: %w", err)
	}
	plist := filepath.Join(app, "Contents", "Info.plist")
	deletePlistKey(plist, "CFBundleIconName")
	for _, kv := range [][2]string{
		{"CFBundleName", Name},
		{"CFBundleDisplayName", Name},
		{"CFBundleIdentifier", bundleID},
		{"CFBundleIconFile", "applet"},
	} {
		setPlistKey(plist, kv[0], kv[1])
	}
	// Everything above invalidates the signature osacompile applied, which
	// macOS reports as a modified bundle and answers by refusing to draw the
	// icon. Re-sign ad hoc, after the last edit.
	resign(app)
	return nil
}

// resign applies an ad-hoc signature. Best effort: the bundle still launches
// unsigned, it just keeps the stock icon until this succeeds.
func resign(app string) {
	if _, err := os.Stat(codesignPath); err != nil {
		return
	}
	_ = exec.Command(codesignPath, "--force", "--sign", "-", app).Run()
}

// deletePlistKey drops a key, tolerating its absence.
func deletePlistKey(plist, key string) {
	if _, err := os.Stat(plistBuddyPath); err != nil {
		return
	}
	_ = exec.Command(plistBuddyPath, "-c", "Delete :"+key, plist).Run()
}

// setPlistKey sets a key, adding it when the compiled plist has none. Best
// effort: a bundle that keeps osacompile's generic name still launches.
func setPlistKey(plist, key, value string) {
	if _, err := os.Stat(plistBuddyPath); err != nil {
		return
	}
	if err := exec.Command(plistBuddyPath, "-c", "Set :"+key+" "+value, plist).Run(); err == nil {
		return
	}
	_ = exec.Command(plistBuddyPath, "-c", "Add :"+key+" string "+value, plist).Run()
}

// registerBundle tells LaunchServices about the bundle now rather than whenever
// the next volume scan gets to it, so the app shows up in Launchpad, Spotlight
// and Open With as soon as the install finishes. Best effort: an unregistered
// bundle still launches from Finder, so a missing or moved tool is not an error.
func registerBundle(app string) {
	if _, err := os.Stat(lsregisterPath); err != nil {
		return
	}
	_ = exec.Command(lsregisterPath, "-f", app).Run()
}

// Remove deletes the launcher bundle. Missing is success, and so is an app of
// another kind at that path, such as the desktop app, which is left alone.
func Remove() error {
	app := Path()
	if app == "" || !replaceable(app) {
		return nil
	}
	return os.RemoveAll(app)
}

// replaceable is true when nothing is at app or what is there is the launcher.
// The desktop app is also called Lerd and can be installed to ~/Applications.
func replaceable(app string) bool {
	if _, err := os.Stat(app); os.IsNotExist(err) {
		return true
	}
	out, err := exec.Command(plistBuddyPath, "-c", "Print :CFBundleIdentifier", filepath.Join(app, "Contents", "Info.plist")).Output()
	return err == nil && strings.TrimSpace(string(out)) == bundleID
}
