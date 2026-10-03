//go:build windows

package workerheal

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// lastNonBlankLine returns the final non-empty line of path, walking from
// the end so a multi-megabyte log doesn't load into memory. The 1 MiB
// trailing window comfortably fits even pathological PHP fatal stack traces
// and matches the scanner buffer ceiling so a single oversize line never gets
// silently dropped via bufio.ErrTooLong.
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
