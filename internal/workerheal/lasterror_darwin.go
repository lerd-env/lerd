//go:build darwin

package workerheal

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// readLastErrorPlatform tails the launchd log file for a unit. The lerd
// service-manager redirects stdout+stderr from each plist to
// ~/Library/Logs/lerd/<unit>.log (see services/launchd_darwin.go), so this
// is the macOS analogue of `journalctl -u <unit> -n 1`. Returns "" when the
// file doesn't exist or contains no usable lines.
func readLastErrorPlatform(unit string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, "Library", "Logs", "lerd", unit+".log")
	return lastNonBlankLine(path)
}

// lastNonBlankLine returns the final non-empty line of path, walking from
// the end so a multi-megabyte log doesn't load into memory. The 64 KiB
// trailing window is more than enough for one log line under any sane
// PHP / supervisor config — runtime stack traces top out well below that.
func lastNonBlankLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return ""
	}
	const window = 64 * 1024
	start := info.Size() - window
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	last := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			last = line
		}
	}
	return last
}
