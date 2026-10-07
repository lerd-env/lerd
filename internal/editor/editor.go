// Package editor resolves the command that opens a path in the user's editor,
// shared by the dashboard's "open in editor" links and the `lerd code` command.
package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/platform"
)

// Editor is one editor lerd knows how to open a file in: the binary that
// jumps to file:line, and the URL scheme the editor registers with the desktop,
// which is how it is reached when its binary is not on PATH (a JetBrains IDE
// installed through Toolbox, say). The ID is what a site's `editor` names.
type Editor struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	bin      string
	lineArgs func(file string, line int) []string
	url      string
	// apps are the macOS bundle names the editor installs as.
	apps []string
	// flatpaks are the Flathub app IDs, which is all a Flatpak install puts on
	// PATH; run with arguments they behave as the editor's own binary.
	flatpaks []string
}

// binary finds the editor's command on PATH, under its own name or a Flatpak ID.
func (e Editor) binary() (string, bool) {
	for _, name := range append([]string{e.bin}, e.flatpaks...) {
		if p, err := exec.LookPath(name); err == nil {
			return p, true
		}
	}
	return "", false
}

func gotoArgs(f string, l int) []string { return []string{"-g", loc(f, l)} }
func locArgs(f string, l int) []string  { return []string{loc(f, l)} }
func jetbrainsArgs(f string, l int) []string {
	return []string{"--line", strconv.Itoa(l), f}
}

// Editors is the curated list, probed on PATH in this order when nothing is
// configured. They open a directory as a bare argument, which DirCommand uses.
var Editors = []Editor{
	{"vscode", "Visual Studio Code", "code", gotoArgs, "vscode://file/{file}:{line}", []string{"Visual Studio Code.app"}, []string{"com.visualstudio.code"}},
	{"cursor", "Cursor", "cursor", gotoArgs, "cursor://file/{file}:{line}", []string{"Cursor.app"}, nil},
	{"vscodium", "VSCodium", "codium", gotoArgs, "vscodium://file/{file}:{line}", []string{"VSCodium.app"}, []string{"com.vscodium.codium"}},
	{"windsurf", "Windsurf", "windsurf", gotoArgs, "windsurf://file/{file}:{line}", []string{"Windsurf.app"}, nil},
	{"sublime", "Sublime Text", "subl", locArgs, "subl://open?url=file://{file}&line={line}", []string{"Sublime Text.app"}, []string{"com.sublimetext.three"}},
	{"zed", "Zed", "zed", locArgs, "zed://file/{file}:{line}", []string{"Zed.app"}, []string{"dev.zed.Zed"}},
	{"phpstorm", "PhpStorm", "phpstorm", jetbrainsArgs, "phpstorm://open?file={file}&line={line}", []string{"PhpStorm.app"}, []string{"com.jetbrains.PhpStorm"}},
	{"idea", "IntelliJ IDEA", "idea", jetbrainsArgs, "idea://open?file={file}&line={line}", []string{"IntelliJ IDEA.app", "IntelliJ IDEA Ultimate.app", "IntelliJ IDEA CE.app"}, []string{"com.jetbrains.IntelliJ-IDEA-Ultimate", "com.jetbrains.IntelliJ-IDEA-Community"}},
	{"webstorm", "WebStorm", "webstorm", jetbrainsArgs, "webstorm://open?file={file}&line={line}", []string{"WebStorm.app"}, []string{"com.jetbrains.WebStorm"}},
}

// Installed reports whether the editor is on this machine: its binary on PATH,
// a desktop entry claiming its URL scheme (how JetBrains Toolbox and Flatpak
// install them on Linux), or its app bundle on macOS.
func (e Editor) Installed() bool {
	if _, ok := e.binary(); ok {
		return true
	}
	return e.installedOffPath()
}

// applicationDirs are where desktop entries live; a variable so a test can keep
// the machine's own out.
var applicationDirs = defaultApplicationDirs

func defaultApplicationDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".local/share/applications"),
		"/usr/share/applications",
		"/usr/local/share/applications",
		filepath.Join(home, ".local/share/flatpak/exports/share/applications"),
		"/var/lib/flatpak/exports/share/applications",
	}
}

// schemeHandled reports whether a desktop entry in the usual application dirs
// declares itself the handler of a URL scheme.
func schemeHandled(scheme string) bool {
	want := "x-scheme-handler/" + scheme
	for _, dir := range applicationDirs() {
		entries, _ := filepath.Glob(filepath.Join(dir, "*.desktop"))
		for _, f := range entries {
			if b, err := os.ReadFile(f); err == nil && strings.Contains(string(b), want) {
				return true
			}
		}
	}
	return false
}

// Known returns the curated editor with this ID.
func Known(id string) (Editor, bool) {
	for _, e := range Editors {
		if e.ID == id {
			return e, true
		}
	}
	return Editor{}, false
}

// For resolves how to open file at line in the chosen editor: a listed one by
// its binary when that is on PATH, otherwise by its URL, for the dashboard to
// hand to the desktop. A URL template is filled in the same way; anything else,
// including no choice at all, resolves as Command does.
func For(file string, line int) (argv []string, url string, err error) {
	choice := configuredTemplate()
	e, ok := Known(choice)
	if !ok {
		if IsURLTemplate(choice) {
			return nil, fillTemplate(choice, file, line), nil
		}
		return Command(file, line), "", nil
	}
	if p, ok := e.binary(); ok {
		return append([]string{p}, e.lineArgs(file, line)...), "", nil
	}
	return nil, fillTemplate(e.url, file, line), nil
}

// IsTemplate reports whether a choice is a custom editor rather than a listed
// one: a command or a URL naming where the file goes with {file}.
func IsTemplate(choice string) bool { return strings.Contains(choice, "{file}") }

// IsURLTemplate reports whether a custom editor is reached by its URL scheme.
func IsURLTemplate(choice string) bool { return IsTemplate(choice) && strings.Contains(choice, "://") }

// Valid reports whether the setting may choose this editor: a listed one, a
// custom template, or none.
func Valid(choice string) bool {
	_, ok := Known(choice)
	return choice == "" || ok || IsTemplate(choice)
}

func fillTemplate(tmpl, file string, line int) string {
	return strings.NewReplacer("{file}", file, "{line}", strconv.Itoa(line)).Replace(tmpl)
}

func loc(file string, line int) string { return fmt.Sprintf("%s:%d", file, line) }

// configuredTemplate returns the `editor` command from the global config, or an
// empty string when the user has not set one.
func configuredTemplate() string {
	cfg, _ := config.LoadGlobal()
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Editor)
}

// Command resolves the argv to open file at line. A configured `editor` template
// wins (with {file}/{line} substitution, or the file appended when it has
// neither placeholder); otherwise the first GUI editor found on PATH is used,
// falling back to the platform opener.
func Command(file string, line int) []string {
	if tmpl := configuredTemplate(); tmpl != "" {
		if strings.Contains(tmpl, "{file}") || strings.Contains(tmpl, "{line}") {
			tmpl = strings.ReplaceAll(tmpl, "{file}", file)
			tmpl = strings.ReplaceAll(tmpl, "{line}", strconv.Itoa(line))
			return strings.Fields(tmpl)
		}
		return append(strings.Fields(tmpl), file)
	}

	for _, e := range Editors {
		if p, ok := e.binary(); ok {
			return append([]string{p}, e.lineArgs(file, line)...)
		}
	}
	// Last resort: hand the file to the platform opener (uses the default app).
	if p, err := exec.LookPath(platform.Current.Opener); err == nil {
		return []string{p, file}
	}
	return nil
}

// DirCommand resolves the argv to open a directory, the project-shaped variant
// of Command. A configured template is reused with {file} as the directory and
// {line} dropped, since a directory has no line to jump to. There is no
// platform-opener fallback: xdg-open would hand the directory to the file
// manager rather than an editor, so nil means "no editor found" and the caller
// should say so.
func DirCommand(dir string) []string {
	id := configuredTemplate()
	// A listed editor opens the directory with its binary, or failing that
	// however its platform installs it off PATH.
	if e, ok := Known(id); ok {
		if p, ok := e.binary(); ok {
			return []string{p, dir}
		}
		return e.dirCommandOffPath(dir)
	}
	if IsURLTemplate(id) {
		return nil
	}
	if tmpl := id; tmpl != "" {
		if strings.Contains(tmpl, "{file}") || strings.Contains(tmpl, "{line}") {
			return dropLinePlaceholder(strings.Fields(tmpl), dir)
		}
		return append(strings.Fields(tmpl), dir)
	}
	for _, e := range Editors {
		if p, ok := e.binary(); ok {
			return []string{p, dir}
		}
	}
	return nil
}

// dropLinePlaceholder substitutes {file} with dir and removes {line} from a
// template's fields. A field that is only the placeholder takes the flag in
// front of it with it ("--line {line} {file}"), and one that carries it after a
// separator keeps the file and loses the tail ("-g {file}:{line}").
func dropLinePlaceholder(fields []string, dir string) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if !strings.Contains(f, "{line}") {
			out = append(out, strings.ReplaceAll(f, "{file}", dir))
			continue
		}
		trimmed := strings.TrimRight(strings.SplitN(f, "{line}", 2)[0], ":,+ \t")
		if trimmed == "" {
			// The placeholder stood alone, so the flag introducing it is orphaned.
			if n := len(out); n > 0 && strings.HasPrefix(out[n-1], "-") {
				out = out[:n-1]
			}
			continue
		}
		out = append(out, strings.ReplaceAll(trimmed, "{file}", dir))
	}
	return out
}
