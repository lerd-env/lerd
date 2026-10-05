package ui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// terminalScriptUTI is the file type of a .command script. The app LaunchServices
// records as its handler is what macOS means by the default terminal, and it is
// what "Open in terminal" should reach for before any emulator that merely
// happens to be on PATH.
const terminalScriptUTI = "com.apple.terminal.shell-script"

// launchServicesPlist is where the user's own handler choices live. The system
// defaults are not written here, so an absent file or a missing entry both mean
// the user never changed their terminal.
const launchServicesPlist = "Library/Preferences/com.apple.LaunchServices/com.apple.launchservices.secure.plist"

// macDefaultTerminalBundle reads the bundle id LaunchServices records for
// terminal scripts out of the plist's JSON form. It returns "" when the user
// never picked a terminal, which leaves the caller on its ordinary fallbacks.
func macDefaultTerminalBundle(plistJSON []byte) string {
	var doc struct {
		Handlers []struct {
			ContentType string `json:"LSHandlerContentType"`
			RoleAll     string `json:"LSHandlerRoleAll"`
			RoleShell   string `json:"LSHandlerRoleShell"`
		} `json:"LSHandlers"`
	}
	if err := json.Unmarshal(plistJSON, &doc); err != nil {
		return ""
	}
	for _, h := range doc.Handlers {
		if h.ContentType != terminalScriptUTI {
			continue
		}
		// A terminal claims the shell role, but the picker in Finder writes the
		// catch-all one, so take whichever the entry carries.
		if h.RoleAll != "" {
			return h.RoleAll
		}
		return h.RoleShell
	}
	return ""
}

// kdeDefaultTerminal reads TerminalApplication out of a kdeglobals body. The
// file is INI, and the key lives under [General].
func kdeDefaultTerminal(body []byte) string {
	section := ""
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line
			continue
		}
		if section != "[General]" {
			continue
		}
		if v, ok := strings.CutPrefix(line, "TerminalApplication="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// gnomeDefaultTerminal reads GNOME's own key. The value is quoted, and it can
// name a launcher that is not installed, which the caller's LookPath catches.
func gnomeDefaultTerminal() string {
	out, err := exec.Command("gsettings", "get",
		"org.gnome.desktop.default-applications.terminal", "exec").Output()
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(string(out)), "'\"")
}

// readHomeFile reads a path relative to the home directory, empty on any failure.
func readHomeFile(rel string) []byte {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(home, rel))
	if err != nil {
		return nil
	}
	return body
}
