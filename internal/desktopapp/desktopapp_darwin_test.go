package desktopapp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func withAppsDir(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(osacompilePath); err != nil {
		t.Skip("osacompile is not available on this host")
	}
	dir := t.TempDir()
	origApps, origLS, origApp := appsDir, lsregisterPath, desktopAppInstalled
	t.Cleanup(func() { appsDir = origApps; lsregisterPath = origLS; desktopAppInstalled = origApp })
	appsDir = func() string { return dir }
	// The host may have the desktop app, which would make Install step aside.
	desktopAppInstalled = func() bool { return false }
	// Keep the real LaunchServices database out of the tests.
	lsregisterPath = filepath.Join(dir, "no-such-lsregister")
	return dir
}

func TestInstallBuildsALaunchableApplication(t *testing.T) {
	dir := withAppsDir(t)

	app, err := Install()
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if want := filepath.Join(dir, "Lerd.app"); app != want {
		t.Errorf("Install() = %q, want %q", app, want)
	}

	exe := filepath.Join(app, "Contents", "MacOS", "applet")
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("stat the applet: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Errorf("applet mode = %v, want executable", info.Mode().Perm())
	}

	// osacompile writes a nameless script application; without this the app
	// shows up in Launchpad and the Dock as "applet".
	plist, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatalf("reading Info.plist: %v", err)
	}
	for _, want := range []string{"Lerd", bundleID} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("Info.plist is missing %q", want)
		}
	}

	if got, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "applet.icns")); err != nil {
		t.Errorf("reading the icon: %v", err)
	} else if len(got) != len(iconICNS) {
		t.Error("the applet kept osacompile's generic icon")
	}
	// The splash draws from a PNG: NSImage mattes an icns representation to
	// white at the size the window uses, which put a box behind the mark.
	if got, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "lerd-mark.png")); err != nil {
		t.Errorf("reading the splash mark: %v", err)
	} else if len(got) != len(markPNG) {
		t.Error("the splash mark was not written")
	}
}

// Writing applet.icns is not enough on its own: macOS prefers the compiled
// asset catalog, so the icon drawn stays osacompile's stock one until both the
// catalog and the key pointing at it are gone.
func TestInstallLeavesNothingOutrankingTheIcon(t *testing.T) {
	withAppsDir(t)

	app, err := Install()
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(app, "Contents", "Resources", "Assets.car")); !os.IsNotExist(err) {
		t.Errorf("the stock asset catalog survived: %v", err)
	}
	plist, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatalf("reading Info.plist: %v", err)
	}
	if strings.Contains(string(plist), "CFBundleIconName") {
		t.Error("CFBundleIconName still points at the asset catalog")
	}
	if !strings.Contains(string(plist), "CFBundleIconFile") {
		t.Error("CFBundleIconFile is missing, so nothing names the icns")
	}
}

// Editing a bundle invalidates the signature osacompile applied, and macOS
// answers a modified bundle by refusing to draw its icon.
func TestInstalledBundleIsSignedAfterBeingEdited(t *testing.T) {
	withAppsDir(t)
	if _, err := os.Stat(codesignPath); err != nil {
		t.Skip("codesign is not available on this host")
	}

	app, err := Install()
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if out, err := exec.Command(codesignPath, "-v", app).CombinedOutput(); err != nil {
		t.Errorf("codesign -v failed: %v\n%s", err, out)
	}
}

// The window is the point: a start takes the better part of a minute, and a
// bundle that only runs a shell script has nothing to show for it.
func TestLauncherDrawsProgressAndDefersToTheDashboardCommand(t *testing.T) {
	for _, want := range []string{
		"NSProgressIndicator",
		"NSImageView",
		"lerd-mark.png",
		"on startFinished()",
		"dashboard > /dev/null",
	} {
		if !strings.Contains(launcherScript, want) {
			t.Errorf("launcher is missing %q", want)
		}
	}
	if !strings.Contains(launcherScript, "{{LERD_BIN}}") {
		t.Error("launcher has no placeholder for the resolved binary")
	}
	// A plain delay blocks the main thread, and a window that never gets the
	// run loop back never redraws: the splash would freeze on its first frame.
	if !strings.Contains(launcherScript, "runUntilDate:") {
		t.Error("launcher sleeps instead of yielding to the run loop")
	}
	// Drawing can fail on a host we cannot predict, and a launcher that then
	// runs silently for a minute is worse than one with a plain progress bar.
	if !strings.Contains(launcherScript, "set progress additional description to") {
		t.Error("launcher has no fallback for when the window cannot be built")
	}
}

func TestInstalledLauncherCompilesAndCarriesTheResolvedBinary(t *testing.T) {
	withAppsDir(t)

	app, err := Install()
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	out, err := exec.Command("/usr/bin/osadecompile", filepath.Join(app, "Contents", "Resources", "Scripts", "main.scpt")).CombinedOutput()
	if err != nil {
		t.Skipf("osadecompile unavailable: %v", err)
	}
	if strings.Contains(string(out), "{{LERD_BIN}}") {
		t.Error("the installed launcher still holds the placeholder")
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	withAppsDir(t)

	first, err := Install()
	if err != nil {
		t.Fatalf("first Install() error: %v", err)
	}
	second, err := Install()
	if err != nil {
		t.Fatalf("second Install() error: %v", err)
	}
	if first != second {
		t.Errorf("Install() moved the bundle: %q then %q", first, second)
	}
}

func TestRemoveDeletesTheBundleAndToleratesItsAbsence(t *testing.T) {
	withAppsDir(t)

	app, err := Install()
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if err := Remove(); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if _, err := os.Stat(app); !os.IsNotExist(err) {
		t.Errorf("bundle still present after Remove(): %v", err)
	}
	if err := Remove(); err != nil {
		t.Errorf("second Remove() error: %v", err)
	}
}

// foreignBundle puts an app that is not lerd's launcher at the launcher's path,
// the way the desktop app lands there when /Applications is not writable.
func foreignBundle(t *testing.T) string {
	t.Helper()
	app := Path()
	if err := os.MkdirAll(filepath.Join(app, "Contents"), 0755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>sh.lerd.Desktop</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0644); err != nil {
		t.Fatal(err)
	}
	return app
}

func withDesktopApp(t *testing.T, installed bool) {
	t.Helper()
	prev := desktopAppInstalled
	t.Cleanup(func() { desktopAppInstalled = prev })
	desktopAppInstalled = func() bool { return installed }
}

func TestInstallStepsAsideForTheDesktopApp(t *testing.T) {
	withAppsDir(t)
	withDesktopApp(t, false)
	app, err := Install()
	if err != nil {
		t.Fatal(err)
	}

	withDesktopApp(t, true)
	got, err := Install()
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if got != "" {
		t.Errorf("Install() = %q, want no launcher beside the desktop app", got)
	}
	if _, err := os.Stat(app); !os.IsNotExist(err) {
		t.Errorf("old launcher still present beside the desktop app: %v", err)
	}
}

func TestInstallAndRemoveNeverTouchAnotherApp(t *testing.T) {
	withAppsDir(t)
	withDesktopApp(t, false)
	app := foreignBundle(t)

	if _, err := Install(); err == nil {
		t.Error("Install() replaced an app that is not the launcher")
	}
	if err := Remove(); err != nil {
		t.Errorf("Remove() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(app, "Contents", "Info.plist")); err != nil {
		t.Errorf("the other app was touched: %v", err)
	}
}
