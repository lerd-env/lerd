package podman

import "strings"

// InteractiveShellScript returns the `sh -c '...'` payload that picks an
// interactive shell inside a container. The PHP-FPM image ships zsh with
// a lerd-controlled config (starship prompt, persistent history); other
// images fall through to bash or sh.
func InteractiveShellScript() string {
	return buildShellChain([]string{"zsh", "bash", "sh"})
}

func buildShellChain(shells []string) string {
	var b strings.Builder
	for i, s := range shells {
		if i > 0 {
			b.WriteString(" || ")
		}
		if i == len(shells)-1 {
			b.WriteString("exec ")
			b.WriteString(s)
			continue
		}
		b.WriteString("command -v ")
		b.WriteString(s)
		b.WriteString(" >/dev/null 2>&1 && exec ")
		b.WriteString(s)
	}
	return b.String()
}
