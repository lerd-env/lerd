//go:build linux

package desktopapp

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

//go:embed assets/lerd-mark.png
var markPNG []byte

// LauncherName is what the Linux desktop entry is called. On its own it is
// "Lerd", the same as everywhere else. The lerd-desktop app ships an entry under
// that name too, and shadowing that one is not an option: it carries the
// x-scheme-handler/lerd association, and taking it over would point lerd:// back
// at this launcher, which opens the app through lerd://. So where both are
// installed the two sit side by side and this one says what it adds.
func LauncherName() string {
	if desktopAppEntryInstalled() {
		return "Start " + Name
	}
	return Name
}

// desktopAppEntryID is the lerd-desktop app's entry, named after its app id.
const desktopAppEntryID = "sh.lerd.Desktop.desktop"

// desktopAppEntryInstalled reports whether the desktop app already lists itself.
// The flatpak exports its entry per user and system-wide, and a distro package
// would put one in the shared application directories, so all of them are asked.
func desktopAppEntryInstalled() bool {
	dirs := []string{
		filepath.Join(dataHome(), "flatpak", "exports", "share", "applications"),
		"/var/lib/flatpak/exports/share/applications",
	}
	for _, base := range filepath.SplitList(xdgDataDirs()) {
		dirs = append(dirs, filepath.Join(base, "applications"))
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, desktopAppEntryID)); err == nil {
			return true
		}
	}
	return false
}

// xdgDataDirs is the shared data search path, with the spec's default when the
// session sets none.
func xdgDataDirs() string {
	if v := os.Getenv("XDG_DATA_DIRS"); v != "" {
		return v
	}
	return "/usr/local/share:/usr/share"
}

// entryName is the desktop file's basename. It doubles as the icon's, so a
// launcher that resolves Icon= through the theme finds the same artwork.
const entryName = "lerd"

// dataHome is the XDG base for the desktop entry and the icon, held in a var so
// a test can point it somewhere harmless.
var dataHome = func() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share")
}

// Path is the desktop entry, empty when there is nowhere to write it.
func Path() string {
	dir := dataHome()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "applications", entryName+".desktop")
}

// iconPath is where the entry's Icon= points. An absolute path rather than a
// theme name: a themed icon has to land in the right size directory to be found
// at all, and the mark ships at one size.
func iconPath() string {
	return filepath.Join(dataHome(), entryName, entryName+".png")
}

// Install writes the desktop entry and its icon, and returns the entry's path.
// Rewritten on every run so an upgraded lerd repoints it at the binary that is
// live now.
//
// The entry runs `lerd dashboard --splash`, which is the same command the tray
// and the CLI use: it starts the environment when it is down, opens the desktop
// app when that is installed and the browser otherwise. --splash is what makes
// it usable from an icon, where there is no terminal to show progress in.
func Install() (string, error) {
	entry := Path()
	if entry == "" {
		return "", fmt.Errorf("locating the home directory")
	}
	icon := iconPath()
	if err := os.MkdirAll(filepath.Dir(icon), 0755); err != nil {
		return "", fmt.Errorf("creating %s: %w", filepath.Dir(icon), err)
	}
	if err := os.WriteFile(icon, markPNG, 0644); err != nil {
		return "", fmt.Errorf("writing the icon: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(entry), 0755); err != nil {
		return "", fmt.Errorf("creating %s: %w", filepath.Dir(entry), err)
	}
	if err := os.WriteFile(entry, []byte(desktopEntry(config.LerdBinary(), icon)), 0644); err != nil {
		return "", fmt.Errorf("writing the desktop entry: %w", err)
	}
	refreshDesktopDatabase(filepath.Dir(entry))
	return entry, nil
}

// desktopEntry renders the .desktop file. Exec is the resolved binary rather
// than a bare name: a launcher starts it with a minimal PATH that need not carry
// ~/.local/bin.
func desktopEntry(bin, icon string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=" + LauncherName() + "\n" +
		"GenericName=Local PHP development environment\n" +
		"Comment=Bring the environment up, then open the Lerd app or the dashboard\n" +
		"Exec=" + bin + " dashboard --splash\n" +
		"Icon=" + icon + "\n" +
		"Terminal=false\n" +
		"Categories=Development;\n" +
		"Keywords=php;laravel;podman;lerd;\n" +
		"StartupNotify=true\n"
}

// refreshDesktopDatabase asks the desktop to notice the new entry now rather
// than at the next login. Best effort: the entry is readable either way.
func refreshDesktopDatabase(dir string) {
	bin, err := exec.LookPath("update-desktop-database")
	if err != nil {
		return
	}
	_ = exec.Command(bin, dir).Run()
}

// windowEntryMarker tags the hidden entries WriteWindowEntry leaves, so Remove
// can find them without knowing which browsers were used.
const windowEntryMarker = "X-Lerd-App-Window=true"

// WriteWindowEntry names a dashboard app window for the desktop. A Chromium
// --app window's class is derived from its URL, and a taskbar looks the icon up
// in the entry of that name, so without one the window shows a generic icon.
// The entry is hidden: it exists to be matched, not launched.
func WriteWindowEntry(class string) error {
	dir := dataHome()
	if dir == "" {
		return fmt.Errorf("locating the home directory")
	}
	icon := iconPath()
	if err := os.MkdirAll(filepath.Dir(icon), 0755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(icon), err)
	}
	if err := os.WriteFile(icon, markPNG, 0644); err != nil {
		return fmt.Errorf("writing the icon: %w", err)
	}
	apps := filepath.Join(dir, "applications")
	if err := os.MkdirAll(apps, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", apps, err)
	}
	body := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=" + Name + "\n" +
		"Exec=" + config.LerdBinary() + " dashboard --splash\n" +
		"Icon=" + icon + "\n" +
		"StartupWMClass=" + class + "\n" +
		"NoDisplay=true\n" +
		windowEntryMarker + "\n"
	return os.WriteFile(filepath.Join(apps, class+".desktop"), []byte(body), 0644)
}

// removeWindowEntries deletes every entry WriteWindowEntry left.
func removeWindowEntries(apps string) {
	entries, _ := filepath.Glob(filepath.Join(apps, "*.desktop"))
	for _, path := range entries {
		body, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(body), windowEntryMarker) {
			_ = os.Remove(path)
		}
	}
}

// Remove deletes the desktop entry and its icon. Missing is success.
//
// Only the icon file: it sits directly in lerd's data directory, so removing
// its parent takes the whole of ~/.local/share/lerd with it, which an uninstall
// that was told to keep the data has no business doing.
func Remove() error {
	entry := Path()
	if entry == "" {
		return nil
	}
	if err := os.Remove(entry); err != nil && !os.IsNotExist(err) {
		return err
	}
	removeWindowEntries(filepath.Dir(entry))
	_ = os.Remove(iconPath())
	refreshDesktopDatabase(filepath.Dir(entry))
	return nil
}
