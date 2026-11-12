package config

import (
	"fmt"
	"regexp"
)

// databaseNamePattern is the shape a name must have to reach a path, SQL or
// shell sink: interior dots stay for S3 buckets, while separators, a leading
// dot and every shell or SQL metacharacter are out.
var databaseNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// maxDatabaseNameLen is MySQL's identifier limit, the tighter of the two engines.
const maxDatabaseNameLen = 64

// ValidateDatabaseName guards the sinks that assume a slugged name: snapshot
// paths, information_schema lookups and the shell commands presets expand.
func ValidateDatabaseName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("a database name is required")
	case len(name) > maxDatabaseNameLen:
		return fmt.Errorf("database name is longer than %d characters", maxDatabaseNameLen)
	case !databaseNamePattern.MatchString(name):
		return fmt.Errorf("invalid database name %q: use letters, digits, underscores and dashes only", name)
	}
	return nil
}
