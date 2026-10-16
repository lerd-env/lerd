package wsl

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// startMenuFn returns the Linux path of the Windows user's Start Menu programs
// folder, or "" without interop; a var so tests use a fixture.
var startMenuFn = func() string {
	cmd := exec.Command(WindowsTool("cmd.exe"), "/c", "echo %APPDATA%")
	cmd.Dir = "/"
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	p, err := exec.Command("wslpath", "-u", strings.TrimSpace(string(out))).Output()
	if err != nil {
		return ""
	}
	return filepath.Join(strings.TrimSpace(string(p)), "Microsoft", "Windows", "Start Menu", "Programs")
}

// appIDArg is "--app-id=" as UTF-16LE, the encoding a .lnk keeps its arguments in.
var appIDArg = func() []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune("--app-id=")) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}()

// PWAShortcut finds the Start Menu shortcut a Chromium browser (Edge, Chrome,
// Vivaldi, Brave) makes when the dashboard is installed as an app: a Lerd.lnk
// launched with --app-id, in a folder named after the browser and translated.
// explorer.exe hands it to the shell, which outlives the WSL session.
func PWAShortcut() (string, bool) {
	root := startMenuFn()
	if root == "" {
		return "", false
	}
	var found string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck
		if err != nil || d.IsDir() || !strings.EqualFold(d.Name(), "Lerd.lnk") {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, appIDArg) {
			found = p
			return fs.SkipAll
		}
		return nil
	})
	return found, found != ""
}
