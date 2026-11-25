package node

import (
	"slices"
	"strings"
)

// Missing reports whether the active manager has no install satisfying version. A
// major or partial version is met by any release under it, so asking never pulls a
// newer patch. Aliases such as lts have nothing to compare and count as present.
func Missing(version string) bool {
	v := strings.TrimPrefix(SafeVersion(version), "v")
	if !isNumericVersion(extractMajor(v)) {
		return false
	}
	return !slices.ContainsFunc(Active().ListFull(), func(full string) bool {
		return full == v || strings.HasPrefix(full, v+".")
	})
}
