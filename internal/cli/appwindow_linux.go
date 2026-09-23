//go:build linux

package cli

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/desktopapp"
	"github.com/godbus/dbus/v5"
)

// chromiumBrowser is a browser that honours --app: the prefix it gives an app
// window's class, then its binary names and its flatpak id, which is also what
// its exported launcher is called.
type chromiumBrowser struct {
	brand string
	names []string
}

var chromiumBrowsers = []chromiumBrowser{
	{"chrome", []string{"chromium", "chromium-browser", "org.chromium.Chromium"}},
	{"chrome", []string{"google-chrome-stable", "google-chrome", "com.google.Chrome"}},
	{"brave", []string{"brave", "brave-browser", "com.brave.Browser"}},
	{"msedge", []string{"microsoft-edge-stable", "microsoft-edge", "com.microsoft.Edge"}},
	{"vivaldi", []string{"vivaldi-stable", "vivaldi", "com.vivaldi.Vivaldi"}},
	{"helium", []string{"helium"}},
}

// openDashboard opens the dashboard in a Chromium --app window, which has no
// tab strip or address bar, the way Omarchy launches its web apps. A PWA
// install still draws Chromium's own title bar, so this is the only way to a
// clean window. Without a Chromium browser it is an ordinary browser tab.
//
// Chromium opens a fresh --app window on every call, so a window already open
// is focused instead where the compositor lets us find it.
func openDashboard(dashURL string) error {
	key := appWindowKey(dashURL)
	if focusAppWindow(key) {
		return nil
	}
	out, _ := exec.Command("xdg-settings", "get", "default-web-browser").Output()
	cmd, brand := appWindowCommand(dashURL, strings.TrimSpace(string(out)), findBrowser)
	if cmd == nil {
		return openBrowser(dashURL)
	}
	// Best effort: without it the window opens all the same, with a generic icon.
	_ = desktopapp.WriteWindowEntry(brand + "-" + key + "-Default")
	return exec.Command(cmd[0], cmd[1:]...).Start()
}

// appWindowCommand picks the browser to open url in as an app window, the
// default browser first when it is one that can, and returns the command with
// the browser's class prefix, or nil when none is installed.
func appWindowCommand(url, defaultBrowser string, find func(string) string) ([]string, string) {
	id := strings.TrimSuffix(defaultBrowser, ".desktop")
	order := chromiumBrowsers
	for _, b := range chromiumBrowsers {
		if slices.Contains(b.names, id) {
			order = append([]chromiumBrowser{b}, chromiumBrowsers...)
			break
		}
	}
	for _, b := range order {
		for _, name := range b.names {
			if path := find(name); path != "" {
				return []string{path, "--app=" + url}, b.brand
			}
		}
	}
	return nil, ""
}

// findBrowser resolves a name on PATH, then among the flatpak exports, which a
// launcher's PATH need not carry.
func findBrowser(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	dirs := []string{"/var/lib/flatpak/exports/bin"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append([]string{filepath.Join(home, ".local", "share", "flatpak", "exports", "bin")}, dirs...)
	}
	for _, dir := range dirs {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// appWindowKey is the part of an --app window's class that names its site:
// Chromium calls it <brand>-<host>_<path with / as _>-<profile>, dropping the
// port, so the dashboard's root page is <host>__ whatever the browser.
func appWindowKey(dashURL string) string {
	u, err := url.Parse(dashURL)
	if err != nil {
		return ""
	}
	return u.Hostname() + "__"
}

// focusAppWindow raises an open --app window whose class carries key, and
// reports whether there was one. Hyprland and KWin are asked; anywhere else a
// window is never found and a new one opens.
func focusAppWindow(key string) bool {
	if key == "" {
		return false
	}
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" {
		return focusHyprlandWindow(key)
	}
	if strings.Contains(os.Getenv("XDG_CURRENT_DESKTOP"), "KDE") {
		return focusKWinWindow(key)
	}
	return false
}

func focusHyprlandWindow(key string) bool {
	out, err := exec.Command("hyprctl", "clients", "-j").Output()
	if err != nil {
		return false
	}
	address := hyprlandAppWindow(out, key)
	if address == "" {
		return false
	}
	// Hyprland's Lua config (Omarchy Quattro) takes a Lua dispatch; the classic
	// config only knows focuswindow, and hyprctl exits 0 on either refusal.
	lua := `hl.dsp.focus({ window = "address:` + address + `" })`
	if out, err := exec.Command("hyprctl", "dispatch", lua).Output(); err == nil && strings.TrimSpace(string(out)) == "ok" {
		return true
	}
	return exec.Command("hyprctl", "dispatch", "focuswindow", "address:"+address).Run() == nil
}

// hyprlandAppWindow finds the address of the window whose class carries key in
// `hyprctl clients -j` output, or returns "".
func hyprlandAppWindow(clients []byte, key string) string {
	var windows []struct {
		Address string `json:"address"`
		Class   string `json:"class"`
	}
	if json.Unmarshal(clients, &windows) != nil {
		return ""
	}
	for _, w := range windows {
		if strings.Contains(w.Class, "-"+key) {
			return w.Address
		}
	}
	return ""
}

// focusKWinWindow goes through KWin's window runner, the one behind KRunner's
// window search: it matches the window class, and activating a match raises
// it on whichever desktop it is on.
func focusKWinWindow(key string) bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer conn.Close()
	runner := conn.Object("org.kde.KWin", "/WindowsRunner")
	var matches []struct {
		ID         string
		Text       string
		Icon       string
		Type       int32
		Relevance  float64
		Properties map[string]dbus.Variant
	}
	if runner.Call("org.kde.krunner1.Match", 0, key).Store(&matches) != nil || len(matches) == 0 {
		return false
	}
	return runner.Call("org.kde.krunner1.Run", 0, matches[0].ID, "").Err == nil
}
