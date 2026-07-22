package nginx

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// httpOverrideNames returns the directive names declared at the top level of the
// user snippets in http.d. nginx rejects a duplicate simple directive in the
// same context rather than letting the later one win, so lerd's own http{}
// defaults with these names have to step aside for the override to load.
func httpOverrideNames() map[string]bool {
	names := map[string]bool{}
	paths, err := filepath.Glob(filepath.Join(config.NginxHttpD(), "*.conf"))
	if err != nil {
		return names
	}
	for _, p := range paths {
		body, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		depth := 0
		for _, line := range strings.Split(string(body), "\n") {
			stmt := strings.TrimSpace(stripConfComment(line))
			if stmt == "" {
				depth += braceDelta(stmt)
				continue
			}
			if depth == 0 {
				if name := strings.Trim(strings.Fields(stmt)[0], "{};"); name != "" {
					names[name] = true
				}
			}
			depth += braceDelta(stmt)
			if depth < 0 {
				depth = 0
			}
		}
	}
	return names
}

// repeatableDirectives may appear more than once in the same context, so a
// user declaring one adds to lerd's rather than colliding with it. Dropping
// lerd's log_format leaves access_log naming a format nginx no longer knows.
var repeatableDirectives = map[string]bool{
	"include":    true,
	"log_format": true,
	"access_log": true,
}

// dropOverriddenDefaults comments out every http{}-level directive of the
// rendered nginx.conf whose name the user also declares in http.d. The line
// stays as a comment so the shipped default is still discoverable on disk.
func dropOverriddenDefaults(conf string, names map[string]bool) string {
	if len(names) == 0 {
		return conf
	}
	lines := strings.Split(conf, "\n")
	depth, inHTTP := 0, false
	for i, line := range lines {
		stmt := strings.TrimSpace(stripConfComment(line))
		if inHTTP && depth == 1 && stmt != "" {
			name := strings.Trim(strings.Fields(stmt)[0], "{};")
			if !repeatableDirectives[name] && names[name] {
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				lines[i] = indent + "# " + strings.TrimSpace(line) + "  (overridden in http.d)"
			}
		}
		if !inHTTP && depth == 0 && strings.HasPrefix(stmt, "http") && strings.Contains(stmt, "{") {
			inHTTP = true
		}
		depth += braceDelta(stmt)
		if depth <= 0 {
			depth, inHTTP = 0, false
		}
	}
	return strings.Join(lines, "\n")
}

// stripConfComment drops the trailing "# …" part of an nginx config line.
func stripConfComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		return line[:i]
	}
	return line
}

// braceDelta reports the block-nesting change a config line causes.
func braceDelta(stmt string) int {
	return strings.Count(stmt, "{") - strings.Count(stmt, "}")
}
