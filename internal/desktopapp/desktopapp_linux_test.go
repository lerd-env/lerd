//go:build linux

package desktopapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstall_writesAClickableEntryAndItsIcon(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())

	entry, err := Install()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "applications", "lerd.desktop"); entry != want {
		t.Errorf("entry = %q, want %q", entry, want)
	}
	body, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	// Clicked from a launcher there is no terminal, so the entry has to say so
	// and has to ask for the splash, or the click is a minute of nothing.
	for _, want := range []string{
		"Type=Application", "Name=Lerd", "Terminal=false",
		"dashboard --splash", "Categories=Development;",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("desktop entry is missing %q:\n%s", want, text)
		}
	}
	icon := filepath.Join(dir, "lerd", "lerd.png")
	if !strings.Contains(text, "Icon="+icon) {
		t.Errorf("entry does not point at the installed icon:\n%s", text)
	}
	if info, err := os.Stat(icon); err != nil || info.Size() == 0 {
		t.Errorf("icon not written: %v", err)
	}
}

// Exec names the resolved binary: a launcher starts it with a minimal PATH that
// need not carry ~/.local/bin.
func TestDesktopEntry_execIsAnAbsolutePath(t *testing.T) {
	body := desktopEntry("/home/someone/.local/bin/lerd", "/tmp/lerd.png")
	if !strings.Contains(body, "Exec=/home/someone/.local/bin/lerd dashboard --splash") {
		t.Errorf("Exec line is not the resolved binary:\n%s", body)
	}
}

func TestRemove_takesTheEntryAndTheIcon(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	if _, err := Install(); err != nil {
		t.Fatal(err)
	}
	if err := Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Path()); !os.IsNotExist(err) {
		t.Errorf("desktop entry survived removal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "lerd", "lerd.png")); !os.IsNotExist(err) {
		t.Errorf("icon survived removal: %v", err)
	}
	// Removing twice is what an uninstall on a machine that never installed
	// the entry does, and it must not fail.
	if err := Remove(); err != nil {
		t.Errorf("second Remove = %v, want nil", err)
	}
}

// The icon lives in lerd's data directory, so a Remove that reached for the
// directory rather than the file emptied a site registry, the nginx vhosts and
// the certificates on an uninstall that had been told to keep them.
func TestRemove_leavesTheRestOfTheDataDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	if _, err := Install(); err != nil {
		t.Fatal(err)
	}
	sites := filepath.Join(dir, "lerd", "sites.yaml")
	if err := os.WriteFile(sites, []byte("sites: []\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sites); err != nil {
		t.Errorf("removing the launcher took %s with it: %v", sites, err)
	}
}

// The desktop app ships an entry called Lerd of its own, so a second one under
// that name would put two identical icons in the application list. Where it is
// installed the launcher says what it adds instead.
func TestLauncherName_stepsAsideForTheDesktopApp(t *testing.T) {
	data := t.TempDir()
	shared := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_DATA_DIRS", shared)

	if got := LauncherName(); got != "Lerd" {
		t.Errorf("without the app: got %q, want %q", got, "Lerd")
	}

	exports := filepath.Join(data, "flatpak", "exports", "share", "applications")
	if err := os.MkdirAll(exports, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exports, desktopAppEntryID), []byte("[Desktop Entry]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LauncherName(); got != "Start Lerd" {
		t.Errorf("with the flatpak: got %q, want %q", got, "Start Lerd")
	}
}

// A distro package puts its entry in the shared directories rather than a
// flatpak export, and it collides just the same.
func TestLauncherName_alsoSeesAPackagedApp(t *testing.T) {
	shared := t.TempDir()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_DATA_DIRS", shared)

	apps := filepath.Join(shared, "applications")
	if err := os.MkdirAll(apps, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apps, desktopAppEntryID), []byte("[Desktop Entry]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LauncherName(); got != "Start Lerd" {
		t.Errorf("with a packaged app: got %q, want %q", got, "Start Lerd")
	}
}

func TestWindowEntry_givesAnAppWindowTheLerdIcon(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	const class = "chrome-lerd.localhost__-Default"
	if err := WriteWindowEntry(class); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "applications", class+".desktop"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	// The desktop matches the window to this file by name, so it only has to
	// carry the icon and stay out of the application list.
	for _, want := range []string{"Icon=" + iconPath(), "NoDisplay=true", "StartupWMClass=" + class} {
		if !strings.Contains(text, want) {
			t.Errorf("entry lacks %q:\n%s", want, text)
		}
	}
	if _, err := os.Stat(iconPath()); err != nil {
		t.Errorf("icon not written: %v", err)
	}
}

func TestRemove_takesTheWindowEntriesToo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	if err := WriteWindowEntry("brave-lerd.localhost__-Default"); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "applications", "brave-browser.desktop")
	if err := os.WriteFile(other, []byte("[Desktop Entry]\nName=Brave\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "applications", "brave-lerd.localhost__-Default.desktop")); !os.IsNotExist(err) {
		t.Errorf("window entry survived Remove: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("Remove took an entry that is not lerd's: %v", err)
	}
}
