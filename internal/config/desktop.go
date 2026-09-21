package config

import (
	"os"
	"os/exec"
	"strings"
)

// UIThemeSourceDesktop marks a theme that came from the desktop rather than
// from a file, which is what keeps a remove button off it.
const UIThemeSourceDesktop = "desktop"

// Desktop is the desktop environment the dashboard can dress itself after: the
// colours it publishes, and the directory a change to them shows up in. A zero
// Desktop means there is none to follow.
type Desktop struct {
	Theme      *UITheme
	WatchDir   string
	WatchNames []string
}

// CurrentDesktop returns the one desktop whose colours the dashboard follows.
// The readers run in order of how definite their evidence is: Omarchy keeps a
// state directory nothing else writes and ships full themes, while Plasma and
// GNOME are read off files and schemas any machine may happen to carry.
func CurrentDesktop() Desktop {
	for _, read := range []func() Desktop{omarchyDesktop, plasmaDesktop, gnomeDesktop} {
		if d := read(); d.Theme != nil {
			return d
		}
	}
	return Desktop{}
}

// DesktopTheme returns the desktop's colours as a dashboard theme, or nil where
// there is no desktop to follow.
func DesktopTheme() *UITheme {
	return CurrentDesktop().Theme
}

// inDesktopSession says whether the session in front of the user is the named
// desktop. XDG_CURRENT_DESKTOP is a colon separated list, and distributions
// prefix their own name to it, as in "ubuntu:GNOME".
func inDesktopSession(name string) bool {
	for _, part := range strings.Split(os.Getenv("XDG_CURRENT_DESKTOP"), ":") {
		if strings.EqualFold(strings.TrimSpace(part), name) {
			return true
		}
	}
	return false
}

// desktopToolOutput runs one of the desktop's own readers and returns what it
// printed, unquoted. A tool that is missing or unhappy is absence rather than an
// error: most machines are not the desktop being asked about.
func desktopToolOutput(tool string, args ...string) string {
	out, err := exec.Command(tool, args...).Output()
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(string(out)), "'")
}
