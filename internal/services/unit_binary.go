package services

import "strings"

// UnitExecBinary returns the program a unit's first ExecStart line runs, or ""
// when the content has none. Shared by the platform readers and by callers that
// need to know what a unit they are about to write would end up running.
func UnitExecBinary(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		args := SplitExecStart(strings.TrimPrefix(line, "ExecStart="))
		if len(args) == 0 {
			return ""
		}
		return args[0]
	}
	return ""
}
