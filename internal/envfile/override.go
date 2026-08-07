package envfile

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// OverrideFile is the personal, gitignored, per-project override file. It is
// plain dotenv syntax: any KEY=VALUE is layered on top of what `lerd env`
// writes, winning over lerd's defaults and computed values.
const OverrideFile = ".env.lerd_override"

// ExternalServicesKey is the one reserved key inside OverrideFile. Its
// comma/space separated value lists services lerd should NOT start or
// provision for this project (you run your own). It is consumed by lerd and
// never written into the project's env file.
const ExternalServicesKey = "LERD_EXTERNAL_SERVICES"

// ReadOverride loads the personal override file from dir. It returns the
// KEY=VALUE overrides (with the reserved external key stripped out) and the set
// of externally-managed service names (lowercased). A missing or unreadable
// file yields two empty, non-nil collections. Values are kept verbatim,
// including any surrounding quotes, so they round-trip into the env file
// unchanged — quotes matter for values with spaces or '#'.
func ReadOverride(dir string) (overrides map[string]string, external map[string]bool) {
	overrides = map[string]string{}
	external = map[string]bool{}

	f, err := os.Open(filepath.Join(dir, OverrideFile))
	if err != nil {
		return overrides, external
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}
		if k == ExternalServicesKey {
			for _, name := range strings.FieldsFunc(strings.Trim(v, `"'`), func(r rune) bool {
				return r == ',' || r == ' ' || r == '\t'
			}) {
				if name = strings.ToLower(name); name != "" {
					external[name] = true
				}
			}
			continue
		}
		overrides[k] = v
	}
	return overrides, external
}
