package services

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

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

// missingBinaryFallback picks what to run when a unit's absolute ExecStart path
// does not exist on this host: the helper of that name beside the running
// binary (lerd-tray.exe next to lerd.exe), else the running binary itself.
func missingBinaryFallback(missing string) string {
	self, err := os.Executable()
	if err != nil {
		return missing
	}
	name := filepath.Base(missing)
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		name += ".exe"
	}
	sibling := filepath.Join(filepath.Dir(self), name)
	if _, err := os.Stat(sibling); err == nil {
		return sibling
	}
	return self
}
