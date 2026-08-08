package sitedoctor

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/envfile"
)

// checkEnvDuplicates reports a key the env file sets more than once, which is a
// question lerd cannot answer for the project: Symfony's dotenv keeps the last
// assignment and Laravel's phpdotenv keeps the first, so the same file means
// different things to different runtimes, and lerd reading the first can
// disagree with the application about which database it is even on.
//
// It is reported rather than resolved. Only the project knows which value it
// meant, and picking one silently is how a site ends up served from a database
// nobody chose.
//
// Only dotenv files are ambiguous this way. A PHP file's duplicate has one
// answer the language decides: a second define() is ignored, and a later
// assignment replaces an earlier one, and lerd reads both the same way PHP
// runs them.
func checkEnvDuplicates(path, envFile, envFormat string) (Check, bool) {
	if envFormat != "" && envFormat != envfile.FormatDotenv {
		return Check{}, false
	}
	dupes := envfile.DuplicateKeys(filepath.Join(path, envFile))
	if len(dupes) == 0 {
		return Check{Name: "env_duplicates", Status: StatusOK}, true
	}

	keys := make([]string, 0, len(dupes))
	for k := range dupes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s is set %d times (lerd reads %q, the last is %q)",
			k, len(dupes[k]), dupes[k][0], dupes[k][len(dupes[k])-1]))
	}
	return Check{
		Name:   "env_duplicates",
		Status: StatusWarn,
		Fix:    FixEnvDuplicates,
		Detail: strings.Join(parts, "; ") + " — in " + envFile +
			", and a framework reading the last value uses a different one than lerd does; keep the one you meant",
	}, true
}
