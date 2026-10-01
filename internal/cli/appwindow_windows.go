//go:build windows

package cli

import (
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// chromiumBrowser is a browser that honours --app: its executable and the
// marker its registered default-browser ProgId carries.
type chromiumBrowser struct {
	exe, progID string
}

// Edge is first because Windows always ships it.
var chromiumBrowsers = []chromiumBrowser{
	{"msedge.exe", "MSEdge"},
	{"chrome.exe", "Chrome"},
	{"brave.exe", "Brave"},
}

// appWindowCommand picks the browser to open url in as an app window, the
// default browser first when it is one that can. Chromium shows the dashboard
// as its installed PWA when there is one, and as a bare window otherwise.
// It returns nil when none is installed.
func appWindowCommand(url, defaultProgID string, find func(string) string) []string {
	order := chromiumBrowsers
	for _, b := range chromiumBrowsers {
		if strings.Contains(defaultProgID, b.progID) {
			order = append([]chromiumBrowser{b}, chromiumBrowsers...)
			break
		}
	}
	for _, b := range order {
		if path := find(b.exe); path != "" {
			return []string{path, "--app=" + url}
		}
	}
	return nil
}

// findBrowser resolves an executable on PATH, then through the App Paths
// registry the browsers' installers fill in.
func findBrowser(exe string) string {
	if path, err := exec.LookPath(exe); err == nil {
		return path
	}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+exe, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		path, _, err := k.GetStringValue("")
		k.Close()
		if err == nil && path != "" {
			return path
		}
	}
	return ""
}

func defaultBrowserProgID() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\Shell\Associations\UrlAssociations\http\UserChoice`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	id, _, _ := k.GetStringValue("ProgId")
	return id
}

// openAppWindow opens url in a Chromium --app window and reports whether it
// could.
func openAppWindow(url string) bool {
	cmd := appWindowCommand(url, defaultBrowserProgID(), findBrowser)
	return cmd != nil && exec.Command(cmd[0], cmd[1:]...).Start() == nil
}
