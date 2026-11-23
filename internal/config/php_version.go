package config

import "regexp"

var phpVersionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// ValidPHPVersion reports whether s is a PHP version in MAJOR.MINOR form,
// rejecting "8,5", plain words and anything that could step out of a path it
// is joined into. The init prompt, the MCP check and the doctor all ask here.
func ValidPHPVersion(s string) bool {
	return phpVersionRe.MatchString(s)
}
