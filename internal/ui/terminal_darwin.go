package ui

import (
	"os"
	"os/exec"
	"path/filepath"
)

// defaultTerminal is empty for scripts on macOS: its chosen terminal is a bundle
// id, and `open -b` takes a file rather than a command to run.
var defaultTerminal = func() string { return "" }

// macDefaultTerminal returns the user's chosen default terminal, or "" when
// there is none to honour. plutil is what turns the binary plist into something
// readable without a cgo dependency on LaunchServices itself.
func macDefaultTerminal() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", filepath.Join(home, launchServicesPlist)).Output()
	if err != nil {
		return ""
	}
	return macDefaultTerminalBundle(out)
}

// osDefaultDirTerminal is the terminal the user picked in System Settings.
func osDefaultDirTerminal(dir string) []terminalCmd {
	if bundle := macDefaultTerminal(); bundle != "" {
		return []terminalCmd{{"open", []string{"-b", bundle, dir}}}
	}
	return nil
}

// osFallbackDirTerminals opens the macOS apps at dir. `open -a Terminal dir`
// opens a new window there without echoing any command, cleaner than `do script
// "cd ... && exec $SHELL"`, which types it visibly into the shell. iTerm2 takes
// the same, and Warp registers public.folder, so it needs none of its warp:// URI.
func osFallbackDirTerminals(dir string) []terminalCmd {
	var out []terminalCmd
	if _, err := os.Stat("/Applications/Warp.app"); err == nil {
		out = append(out, terminalCmd{"open", []string{"-a", "Warp", dir}})
	}
	if _, err := os.Stat("/Applications/iTerm.app"); err == nil {
		out = append(out, terminalCmd{"open", []string{"-a", "iTerm", dir}})
	}
	return append(out, terminalCmd{"open", []string{"-a", "Terminal", dir}})
}

// osFallbackScriptTerminals runs script in iTerm2 when it is installed, then in
// Terminal, both through AppleScript.
func osFallbackScriptTerminals(script string) []terminalCmd {
	var out []terminalCmd
	if _, err := os.Stat("/Applications/iTerm.app"); err == nil {
		as := "tell application \"iTerm2\"\n\tcreate window with default profile\n\ttell current session of current window\n\t\twrite text " + appleScriptStr(script) + "\n\tend tell\nend tell"
		out = append(out, terminalCmd{"osascript", []string{"-e", as}})
	}
	as := "tell application \"Terminal\"\n\tdo script " + appleScriptStr(script) + "\n\tactivate\nend tell"
	return append(out, terminalCmd{"osascript", []string{"-e", as}})
}

// terminalBaseEnv is lerd's own environment: a launchd agent already sits in
// the user's GUI session.
func terminalBaseEnv() []string { return os.Environ() }
