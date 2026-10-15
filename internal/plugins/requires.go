package plugins

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// commandNameRe is a command looked up on PATH, never a path of its own.
var commandNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// Requirements lists what the plugin needs on this machine: the interpreter
// each of its executables names on its first line, and the commands its
// manifest declares under requires:.
func (p *Plugin) Requirements() []string {
	seen := map[string]bool{}
	for _, run := range p.executables() {
		if in := interpreter(filepath.Join(p.Dir, run)); in != "" {
			seen[in] = true
		}
	}
	for _, r := range p.Requires {
		seen[r] = true
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// executables lists every file the manifest runs.
func (p *Plugin) executables() []string {
	var out []string
	for _, c := range p.Commands {
		out = append(out, c.Run)
	}
	for _, c := range p.SiteCommands {
		out = append(out, c.Run)
	}
	for _, run := range p.Hooks {
		out = append(out, run)
	}
	if p.MCP != nil {
		for _, t := range p.MCP.Tools {
			out = append(out, t.Run)
		}
	}
	for _, e := range p.API {
		out = append(out, e.Run)
	}
	for _, run := range p.Provides {
		out = append(out, run)
	}
	return out
}

// interpreter reads the program a script's #! line runs: the command after
// env, or the absolute path otherwise. A binary or a file without one has none.
func interpreter(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	line, _ := bufio.NewReader(f).ReadString('\n')
	if !strings.HasPrefix(line, "#!") {
		return ""
	}
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return ""
	}
	if filepath.Base(fields[0]) != "env" {
		return fields[0]
	}
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "-") || strings.Contains(f, "=") {
			continue
		}
		return f
	}
	return ""
}

// Missing returns the requirements this machine lacks.
func Missing(reqs []string) []string {
	var out []string
	for _, r := range reqs {
		if !installed(r) {
			out = append(out, r)
		}
	}
	return out
}

func installed(req string) bool {
	if filepath.IsAbs(req) {
		return isExecutable(req)
	}
	for _, dir := range filepath.SplitList(searchPath()) {
		if dir != "" && isExecutable(filepath.Join(dir, req)) {
			return true
		}
	}
	return false
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}

// searchPath is the PATH a plugin runs with: lerd's own bin folder first, so
// its php and node are found even from a service started with a bare PATH.
func searchPath() string {
	path := os.Getenv("PATH")
	bin := config.BinDir()
	for _, dir := range filepath.SplitList(path) {
		if dir == bin {
			return path
		}
	}
	return bin + string(os.PathListSeparator) + path
}

func (p *Plugin) checkRequirements() error {
	for _, r := range p.Requires {
		if !commandNameRe.MatchString(r) {
			return fmt.Errorf("requires %q must be a command name, not a path", r)
		}
	}
	for _, run := range p.executables() {
		if filepath.Base(interpreter(filepath.Join(p.Dir, run))) == "php" {
			return fmt.Errorf("%s is a PHP script, and PHP runs in lerd's containers rather than on this machine: write it in shell, Python or Node", run)
		}
	}
	if gone := Missing(p.Requirements()); len(gone) > 0 {
		return fmt.Errorf("needs %s, which this machine does not have", strings.Join(gone, ", "))
	}
	return nil
}
