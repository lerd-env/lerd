package ui

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"
)

// tailFile writes the last n lines of path to out, then follows it as it grows
// until ctx ends. It waits up to a few seconds for the file to appear, since a
// unit's log only exists once the unit has written to it. It is the portable
// replacement for shelling out to tail -f.
func tailFile(ctx context.Context, path string, n int, out io.Writer) error {
	var f *os.File
	for i := 0; ; i++ {
		var err error
		if f, err = os.Open(path); err == nil {
			break
		}
		if i >= 20 {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
		}
	}
	defer f.Close() //nolint:errcheck

	for _, l := range tailLastLines(f, n) {
		if _, err := io.WriteString(out, l+"\n"); err != nil {
			return err
		}
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	r := bufio.NewReader(f)
	var partial string
	for {
		line, err := r.ReadString('\n')
		partial += line
		if err == nil {
			if _, werr := io.WriteString(out, partial); werr != nil {
				return werr
			}
			partial = ""
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// tailLastLines returns up to n trailing lines of f, reading only the final 256 KiB.
func tailLastLines(f *os.File, n int) []string {
	st, err := f.Stat()
	if err != nil || n <= 0 {
		return nil
	}
	const window = 256 << 10
	start := st.Size() - window
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), window)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return lines
}
