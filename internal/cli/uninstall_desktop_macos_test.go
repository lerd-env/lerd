//go:build darwin

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeMacApps gives the test its own application folders and data folder, and
// records whether the app was asked to quit.
func fakeMacApps(t *testing.T) (apps, data string, quit *bool) {
	t.Helper()
	apps, data = t.TempDir(), filepath.Join(t.TempDir(), "Lerd")
	prevDirs, prevData, prevQuit := macDesktopAppDirs, macDesktopAppData, quitMacDesktopApp
	t.Cleanup(func() { macDesktopAppDirs, macDesktopAppData, quitMacDesktopApp = prevDirs, prevData, prevQuit })
	macDesktopAppDirs = func() []string { return []string{apps} }
	macDesktopAppData = func() string { return data }
	quit = new(bool)
	quitMacDesktopApp = func(string) error { *quit = true; return nil }
	return apps, data, quit
}

func writeBundle(t *testing.T, app, id string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + id + `</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
}

// lerd's launcher is also called Lerd.app, so only the app's own id counts.
func TestMacDesktopApp_FindsTheAppAndNotTheLauncher(t *testing.T) {
	apps, _, _ := fakeMacApps(t)
	app := filepath.Join(apps, "Lerd.app")
	if got := macDesktopApp(); got != "" {
		t.Fatalf("found %q in an empty folder", got)
	}
	writeBundle(t, app, "sh.lerd.launcher")
	if got := macDesktopApp(); got != "" {
		t.Errorf("took the launcher %q for the desktop app", got)
	}
	writeBundle(t, app, desktopFlatpakID)
	if got := macDesktopApp(); got != app {
		t.Errorf("macDesktopApp() = %q, want %q", got, app)
	}
}

func TestRemoveMacDesktopApp_QuitsAndRemovesKeepingDataUnlessAsked(t *testing.T) {
	apps, data, quit := fakeMacApps(t)
	app := filepath.Join(apps, "Lerd.app")
	for _, deleteData := range []bool{false, true} {
		writeBundle(t, app, desktopFlatpakID)
		if err := os.MkdirAll(data, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := removeMacDesktopApp(app, deleteData); err != nil {
			t.Fatal(err)
		}
		if !*quit {
			t.Error("the app was not asked to quit before its bundle went")
		}
		if _, err := os.Stat(app); !os.IsNotExist(err) {
			t.Errorf("bundle still there: %v", err)
		}
		if _, err := os.Stat(data); os.IsNotExist(err) == !deleteData {
			t.Errorf("deleteData=%v: data folder present=%v", deleteData, err == nil)
		}
	}
}

// Deleting the bundle under an app that would not quit leaves it running from
// nothing, so a failed quit keeps the app and its data and says so.
func TestRemoveMacDesktopApp_KeepsEverythingWhenTheAppWillNotQuit(t *testing.T) {
	apps, data, _ := fakeMacApps(t)
	quitMacDesktopApp = func(string) error { return errors.New("Lerd is still running") }
	app := filepath.Join(apps, "Lerd.app")
	writeBundle(t, app, desktopFlatpakID)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := removeMacDesktopApp(app, true); err == nil {
		t.Fatal("removal reported success with the app still running")
	}
	for _, p := range []string{app, data} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s removed while the app ran: %v", p, err)
		}
	}
}
