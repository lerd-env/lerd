package config

import (
	"regexp"
	"strings"
)

// versionedName is a service named after its preset and version: mariadb-12-3.
var versionedName = regexp.MustCompile(`^(.*[a-z].*?)-(\d+(?:-\d+)*)$`)

// FromName fills in the preset and version a service's name carries when the
// entry does not spell them out; a preset repeating the name, which lerd used to
// write, does not. The caller checks that preset really produces the name.
func (s ProjectService) FromName() ProjectService {
	repeated := s.Preset == s.Name && s.PresetVersion == ""
	if s.Custom != nil || (s.Preset != "" && !repeated) || PresetExists(s.Name) {
		return s
	}
	if m := versionedName.FindStringSubmatch(s.Name); m != nil {
		s.Preset, s.PresetVersion = m[1], strings.ReplaceAll(m[2], "-", ".")
	}
	return s
}
