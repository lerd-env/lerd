// Package envpass forwards host environment variables an external environment
// provider (a secrets manager, direnv, a shell wrapper) injected into lerd's
// own process on into the one-shot commands lerd runs in a container.
//
// The contract is an allowlist of names, never values, following the shape
// systemd's PassEnvironment=, ssh's SendEnv and sudo's env_keep already use:
// the provider sets LERD_PASSTHROUGH_ENV to the names it injected, glob
// patterns allowed, and lerd forwards each match as a bare `--env NAME` so
// podman reads the value out of lerd's environment and nothing lands in argv
// or on disk.
package envpass

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// EnvVar is the variable an external environment provider sets before it execs
// lerd, holding the names it injected.
const EnvVar = "LERD_PASSTHROUGH_ENV"

// denied are the variables lerd sets for the container itself. Forwarding a
// host value over them either breaks the container outright (PATH, HOME) or
// spoofs signalling lerd does on its own behalf (LERD_*).
var (
	deniedNames    = map[string]bool{"PATH": true, "HOME": true, "COMPOSER_HOME": true}
	deniedPrefixes = []string{"LD_", "LERD_"}
)

var (
	nameRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	patternRE = regexp.MustCompile(`^[A-Za-z0-9_*?]+$`)
)

// Args returns the podman exec flags forwarding the variables the project at
// dir, or the provider that started lerd, asked for. Names that match nothing
// in environ are skipped silently, as they are in systemd.
func Args(dir string, environ []string) []string {
	names := Names(Patterns(dir, environ), environ)
	args := make([]string, 0, len(names)*2)
	for _, n := range names {
		args = append(args, "--env", n)
	}
	return args
}

// Patterns collects the requested name patterns from both sources: the
// provider contract in environ, and env_passthrough in the project's
// .lerd.yaml for people whose provider is direnv or a shell function and who
// have no wrapper command to carry the variable.
func Patterns(dir string, environ []string) []string {
	var out []string
	for _, e := range environ {
		if k, v, ok := strings.Cut(e, "="); ok && k == EnvVar {
			out = append(out, splitList(v)...)
		}
	}
	if dir != "" {
		if cfg, err := config.LoadProjectConfig(dir); err == nil && cfg != nil {
			out = append(out, cfg.EnvPassthrough...)
		}
	}
	return out
}

// Names expands patterns against environ, returning the variable names to
// forward, deduplicated and sorted so an exec argv is stable.
func Names(patterns []string, environ []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range environ {
		name, _, ok := strings.Cut(e, "=")
		if !ok || seen[name] || !nameRE.MatchString(name) || isDenied(name) {
			continue
		}
		if matchesAny(patterns, name) {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func matchesAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if !patternRE.MatchString(p) {
			continue
		}
		if ok, err := path.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}

func isDenied(name string) bool {
	if deniedNames[name] {
		return true
	}
	for _, p := range deniedPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// splitList accepts both separators providers reach for, a comma-separated
// list and a space-separated one.
func splitList(v string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}
