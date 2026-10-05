// Package editor resolves the command that opens a path in the user's editor,
// shared by the dashboard's "open in editor" links and the `lerd code` command.
package editor

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/platform"
)

// knownEditors are the GUI editors probed on PATH when nothing is configured,
// with the arguments each needs to jump to file:line. They open a directory as a
// bare argument, which is what DirCommand uses.
var knownEditors = []struct {
	bin      string
	lineArgs func(file string, line int) []string
}{
	{"code", func(f string, l int) []string { return []string{"-g", loc(f, l)} }},
	{"cursor", func(f string, l int) []string { return []string{"-g", loc(f, l)} }},
	{"codium", func(f string, l int) []string { return []string{"-g", loc(f, l)} }},
	{"windsurf", func(f string, l int) []string { return []string{"-g", loc(f, l)} }},
	{"subl", func(f string, l int) []string { return []string{loc(f, l)} }},
	{"zed", func(f string, l int) []string { return []string{loc(f, l)} }},
	{"phpstorm", func(f string, l int) []string { return []string{"--line", strconv.Itoa(l), f} }},
	{"idea", func(f string, l int) []string { return []string{"--line", strconv.Itoa(l), f} }},
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

	for _, e := range knownEditors {
		if p, err := exec.LookPath(e.bin); err == nil {
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
	if tmpl := configuredTemplate(); tmpl != "" {
		if strings.Contains(tmpl, "{file}") || strings.Contains(tmpl, "{line}") {
			return dropLinePlaceholder(strings.Fields(tmpl), dir)
		}
		return append(strings.Fields(tmpl), dir)
	}
	for _, e := range knownEditors {
		if p, err := exec.LookPath(e.bin); err == nil {
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
