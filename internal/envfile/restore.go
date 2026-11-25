package envfile

import (
	"fmt"
	"os"
	"strings"
)

// ChangedSince returns the backup's value for each of its keys whose value in
// path now differs or is gone. Keys only path has are not its business.
func ChangedSince(path, backup, format string) map[string]string {
	now := Values(path, format)
	changed := map[string]string{}
	for k, old := range Values(backup, format) {
		if v, ok := now[k]; !ok || v != old {
			changed[k] = old
		}
	}
	return changed
}

// RestoreValues puts the backup's values back into path, keeping every key
// added since.
func RestoreValues(path, backup, format string) error {
	changed := ChangedSince(path, backup, format)
	if format == "" || format == FormatDotenv {
		raw, err := rawDotenv(backup)
		if err != nil {
			return err
		}
		for k := range changed {
			v := raw[k]
			if v != "" && (v[0] == '"' || v[0] == '\'') && strings.Count(v, v[:1]) < 2 {
				return fmt.Errorf("%s spans several lines in %s, copy it back by hand", k, backup)
			}
			changed[k] = v
		}
	}
	return ApplyUpdatesIn(path, format, changed)
}

// rawDotenv reads values with their quotes: ApplyUpdates writes them verbatim,
// and a value with spaces breaks without them.
func rawDotenv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if k = strings.TrimSpace(k); !ok || strings.HasPrefix(line, "#") {
			continue
		}
		if _, seen := out[k]; !seen {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out, nil
}
