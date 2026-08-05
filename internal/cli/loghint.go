package cli

import "github.com/geodro/lerd/internal/unitlog"

// unitLogHint returns the shell command a user can run to view recent unit
// logs. The platform split lives in unitlog, so every surface that offers this
// names the same command.
func unitLogHint(unitName string) string { return unitlog.LogHint(unitName) }
