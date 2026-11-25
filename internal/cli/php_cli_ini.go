package cli

import "strings"

// prependPHPIniArgs puts the framework's directives ahead of the caller's args.
// PHP lets a later -d override an earlier one, so a user's explicit -d still wins.
func prependPHPIniArgs(ini, args []string) []string {
	if len(ini) == 0 {
		return args
	}
	out := make([]string, 0, len(ini)+len(args))
	out = append(out, ini...)
	return append(out, args...)
}

// injectPHPIniIntoCommand rewrites a `php ...` shell command to carry the
// directives. Setup steps exec in the container directly rather than through the
// php shim, so they need this; a command that does not start with php is left be.
func injectPHPIniIntoCommand(command string, ini []string) string {
	if len(ini) == 0 {
		return command
	}
	parts := strings.Fields(command)
	if len(parts) == 0 || parts[0] != "php" {
		return command
	}
	rewritten := append([]string{"php"}, ini...)
	return strings.Join(append(rewritten, parts[1:]...), " ")
}
