package reqstats

import "strings"

// StripQueryFragment returns the path portion of a raw request URI, dropping the
// query string and fragment, and defaulting to "/". This is the concrete path we
// keep as a route's openable example, so no query-string values are retained.
func StripQueryFragment(rawURI string) string {
	path := rawURI
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if path == "" {
		path = "/"
	}
	return path
}

// NormalizeRoute turns a method and raw request URI into a stable route key by
// dropping the query string and collapsing id-like path segments to ":id", so
// "/users/123" and "/users/456" aggregate as one route. Dropping the query also
// keeps tokens and other query-string values out of the in-memory window.
func NormalizeRoute(method, rawURI string) string {
	path := StripQueryFragment(rawURI)
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if s != "" && isIDSegment(s) {
			segs[i] = ":id"
		}
	}
	norm := strings.Join(segs, "/")
	if len(norm) > 1 {
		norm = strings.TrimRight(norm, "/")
	}
	if norm == "" {
		norm = "/"
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return norm
	}
	return method + " " + norm
}

// isIDSegment reports whether a path segment looks like an identifier rather than
// a route name: a pure number, a UUID, or a long hex string (hashes, tokens).
// Anything with a non-hex letter (words, api version tags like "v2") is kept.
func isIDSegment(s string) bool {
	if isAllDigits(s) {
		return true
	}
	if isUUID(s) {
		return true
	}
	return len(s) >= 12 && isHex(s)
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
