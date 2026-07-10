package cli

import (
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// The CLI SAPI never reads a project's .user.ini, so a framework whose commands
// exhaust PHP's 128M default declares php.cli_ini, and lerd passes it as -d on
// every PHP process it starts for that project.

// phpIniArgsFor renders the framework's cli_ini as `-d name=value` pairs, sorted
// so the argv is stable. An invalid set yields nothing rather than letting a
// value smuggle a second -d through.
func phpIniArgsFor(fw *config.Framework) []string {
	if fw == nil || len(fw.PHP.CLIIni) == 0 {
		return nil
	}
	if err := config.ValidatePHPIni(fw.PHP.CLIIni); err != nil {
		return nil
	}
	keys := make([]string, 0, len(fw.PHP.CLIIni))
	for k := range fw.PHP.CLIIni {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		args = append(args, "-d", k+"="+fw.PHP.CLIIni[k])
	}
	return args
}

// phpIniArgsForDir resolves the project's framework and renders its cli_ini.
func phpIniArgsForDir(dir string) []string {
	name, ok := config.DetectFrameworkForDir(dir)
	if !ok {
		return nil
	}
	fw, ok := config.GetFrameworkForDir(name, dir)
	if !ok {
		return nil
	}
	return phpIniArgsFor(fw)
}

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
