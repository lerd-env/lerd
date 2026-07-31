package reqstats

import (
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// IsDevServerRequest reports whether a request URI was answered by the site's
// dev server rather than by the app. The dev server answers on the site's own
// domain, so everything it serves arrives in the timing feed looking like a
// route, and the extensions it serves are not the ones the static-asset test
// knows. Everything it serves sits under the prefix lerd gives it, which is how
// a single nginx location carries the modules and the hot-reload socket alike.
func IsDevServerRequest(rawURI string) bool {
	path := StripQueryFragment(rawURI)
	for _, base := range config.DevServerBases() {
		if strings.HasPrefix(path, base) {
			return true
		}
	}
	return false
}
