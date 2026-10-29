package execguard

import (
	"fmt"
	"os"
	"strings"
)

// processStartTime is field 22 of /proc/<pid>/stat, counted after the command
// name, which can itself hold spaces and parentheses.
func processStartTime(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return ""
	}
	fields := strings.Fields(s[i+1:])
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}
