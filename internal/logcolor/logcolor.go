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

// TerminalPassthrough returns the colour environment an attached terminal
// implies for a container exec. podman forwards none of the host environment,
// so COLORTERM has to travel explicitly: Symfony Console and laravel/prompts
// read it first when they pick a colour mode, and without it hex and 256-colour
// styling is flattened to one of sixteen colours. NO_COLOR and FORCE_COLOR ride
// along so a choice made on the host holds inside the container, with NO_COLOR
// winning outright.
//
// TERM is not forwarded. The images carry terminfo for the xterm family only,
// and Symfony matches TERM against a fixed pattern that foot, wezterm and
// friends fail, so the host value would turn colour off for exactly the
// terminals podman's pinned "xterm" keeps working today. A terminal that
// advertises more than sixteen colours gets xterm-256color instead, an entry
// every image has, which puts the eight-bit path back in reach as well.
func TerminalPassthrough(environ []string) []string {
	env := map[string]string{}
	for _, e := range environ {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
		}
	}
	if env["NO_COLOR"] != "" {
		return []string{"NO_COLOR=" + env["NO_COLOR"]}
	}
	var out []string
	if v := env["FORCE_COLOR"]; v != "" {
		out = append(out, "FORCE_COLOR="+v)
	}
	if env["TERM"] == "dumb" {
		return out
	}
	if v := env["COLORTERM"]; v != "" {
		out = append(out, "COLORTERM="+v)
	}
	if wideColorTerminal(env["TERM"], env["COLORTERM"]) {
		out = append(out, "TERM=xterm-256color")
	}
	return out
}

// wideColorTerminal reports whether the attached terminal claims more than the
// sixteen colours podman's pinned TERM leaves room for.
func wideColorTerminal(term, colorterm string) bool {
	if colorterm != "" {
		return true
	}
	return strings.Contains(term, "256color") || strings.Contains(term, "direct")
}

// PodmanExecTerminalArgs returns TerminalPassthrough as `podman exec` flags.
func PodmanExecTerminalArgs(environ []string) []string {
	vars := TerminalPassthrough(environ)
	args := make([]string, 0, len(vars))
	for _, v := range vars {
		args = append(args, "--env="+v)
	}
	return args
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
