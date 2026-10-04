package ui

import "os/exec"

// kdeGlobals is where KDE records the terminal the user picked in System
// Settings. An absent file or key means they never changed it.
const kdeGlobals = ".config/kdeglobals"

// defaultTerminal is the seam tests replace to stand in for the desktop's own
// setting, which is read off the host.
var defaultTerminal = linuxDefaultTerminal

// linuxDefaultTerminal returns the terminal the desktop is configured to use, or
// "" when nothing was ever chosen. The signals are asked in order of how
// deliberate they are: the freedesktop launcher, the distribution's own
// alternative, then the two desktops that keep a setting of their own.
func linuxDefaultTerminal() string {
	// xdg-terminal-exec is the freedesktop entry point: it resolves the user's
	// choice itself and runs a command in it, so where it exists it is the
	// answer rather than a hint towards one.
	if _, err := exec.LookPath("xdg-terminal-exec"); err == nil {
		return "xdg-terminal-exec"
	}
	// Debian and its derivatives keep the choice as an alternatives symlink,
	// which is exactly "the default terminal emulator" on those systems.
	if _, err := exec.LookPath("x-terminal-emulator"); err == nil {
		return "x-terminal-emulator"
	}
	if t := kdeDefaultTerminal(readHomeFile(kdeGlobals)); t != "" {
		return t
	}
	return gnomeDefaultTerminal()
}

// osDefaultDirTerminal is the terminal the desktop is set to use, which lives in
// the freedesktop launcher, the distribution's alternatives link, or the
// desktop's own setting.
func osDefaultDirTerminal(dir string) []terminalCmd {
	if t := linuxDefaultTerminal(); t != "" {
		return []terminalCmd{namedTerminal(t, dir)}
	}
	return nil
}

// Linux has no terminals beyond the known list to fall back on.
func osFallbackDirTerminals(string) []terminalCmd    { return nil }
func osFallbackScriptTerminals(string) []terminalCmd { return nil }

// terminalBaseEnv carries the graphical session keys a spawned emulator needs to
// reach the display, which a systemd user service does not always have.
func terminalBaseEnv() []string { return graphicalEnv() }
