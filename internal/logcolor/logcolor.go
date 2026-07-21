// Package logcolor centralises the environment that makes CLI tools emit ANSI
// colour when lerd runs them with no TTY attached. Workers, UI-run commands and
// container execs all write to a pipe, a log file or the journal, so tools like
// artisan, composer, vite and pest disable colour on their own unless told
// otherwise. The web UI renders the escapes, so we ask for them everywhere.
package logcolor

import (
	"os"
	"strings"
)

// forced lists the signals colour-capable CLIs honour. FORCE_COLOR covers
// Symfony Console (force-color.org) and the Node ecosystem, CLICOLOR_FORCE the
// BSD/Go tools, and TERM keeps libraries that probe the terminal type from
// falling back to "dumb".
var forced = []string{"FORCE_COLOR=1", "CLICOLOR_FORCE=1", "TERM=xterm-256color"}

// Vars returns the KEY=VALUE pairs that force colour, or nothing when the user
// has opted out with NO_COLOR.
func Vars() []string {
	if os.Getenv("NO_COLOR") != "" {
		return nil
	}
	return append([]string(nil), forced...)
}

// PodmanExecArgs returns the flags to add to a `podman exec` invocation.
func PodmanExecArgs() []string {
	vars := Vars()
	args := make([]string, 0, len(vars))
	for _, v := range vars {
		args = append(args, "--env="+v)
	}
	return args
}

// QuadletEnvLines returns systemd `Environment=` lines for a quadlet or unit
// body, newline-terminated so callers can splice them into a template.
func QuadletEnvLines() string {
	var b strings.Builder
	for _, v := range Vars() {
		b.WriteString(`Environment="` + v + "\"\n")
	}
	return b.String()
}

// ShellExports returns `export KEY=VALUE` lines for a generated guard script.
func ShellExports() string {
	var b strings.Builder
	for _, v := range Vars() {
		b.WriteString("export " + v + "\n")
	}
	return b.String()
}

// Environ appends the colour vars to an existing environment slice.
func Environ(base []string) []string {
	return append(append([]string(nil), base...), Vars()...)
}
