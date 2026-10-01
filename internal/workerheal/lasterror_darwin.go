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
// the end so a multi-megabyte log doesn't load into memory. The 1 MiB
// trailing window comfortably fits even pathological PHP fatal stack traces
// (default xdebug.var_display_max_data is 512 KiB) and matches the scanner
// buffer ceiling so a single oversize line never gets silently dropped via
// bufio.ErrTooLong — the Linux journalctl path has no such limit, and
// surfacing an empty LastError on darwin only would defeat the purpose of
// the heal-banner enrichment.
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
	const window = 1 << 20 // 1 MiB
	start := info.Size() - window
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), window)
	last := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			last = line
		}
	}
	return last
}
