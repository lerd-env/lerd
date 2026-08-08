package envfile

import (
	"bufio"
	"os"
	"strings"
)

// DuplicateKeys returns the keys a dotenv file sets more than once, each with
// its values in the order they appear. Commented lines are not settings, so a
// value left behind under a `#` is not a duplicate of the live one.
//
// A file setting a key twice is read differently by different runtimes, and
// both are defensible: Symfony's dotenv parses into an array so the last
// assignment wins, while Laravel loads through phpdotenv's immutable writer,
// which refuses to overwrite a name already set, so the first wins. lerd reads
// the first everywhere. Nothing can be right for both, which is why this exists
// to report the ambiguity rather than to resolve it.
func DuplicateKeys(path string) map[string][]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	order := map[string][]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if k = strings.TrimSpace(k); k == "" {
			continue
		}
		order[k] = append(order[k], strings.Trim(strings.TrimSpace(v), `"'`))
	}
	if scanner.Err() != nil {
		return nil
	}
	for k, vals := range order {
		if len(vals) < 2 {
			delete(order, k)
		}
	}
	if len(order) == 0 {
		return nil
	}
	return order
}
