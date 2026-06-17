package logsource

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/applog"
	"github.com/geodro/lerd/internal/podman"
)

const defaultLines = 50

// maxLines caps Opts.Lines so a caller-supplied value (the MCP logs tool exposes
// it directly) can't drive a huge slice preallocation — make([]Entry, 0, n) with
// a billion would OOM the process, an unrecoverable fatal error. The byte tail is
// already capped at applog.MaxReadBytes, so no real source yields more than this.
const maxLines = 10000

// Opts are the filters applied to a fetch. Empty fields are ignored.
type Opts struct {
	Since string // relative ("15m", "2h30m"), absolute timestamp, or a prior Cursor
	Until string // upper time bound (same formats)
	Grep  string // regex, falling back to a literal substring when it won't compile
	Lines int    // tail cap (default 50)
	Level string // KindFile/monolog only: error, warning, info, debug…
}

// Entry is one normalised log line. Time/Level/Channel are populated when the
// backend exposes them (monolog files, journal); container stdout fills Text only.
type Entry struct {
	Time    string `json:"time,omitempty"`
	Level   string `json:"level,omitempty"`
	Channel string `json:"channel,omitempty"`
	Text    string `json:"text"`
}

// Result is a window of entries in chronological order (oldest first, newest
// last). Cursor is the newest entry's opaque position — pass it back as
// Opts.Since on the next call to fetch only newer lines.
type Result struct {
	Entries   []Entry
	Cursor    string
	Truncated bool
}

// Lines returns just the text of each entry, oldest first.
func (r Result) Lines() []string {
	out := make([]string, len(r.Entries))
	for i, e := range r.Entries {
		out[i] = e.Text
	}
	return out
}

// Read fetches a filtered window from a single source, dispatching on its kind.
func Read(src Source, opts Opts) (Result, error) {
	if opts.Lines <= 0 {
		opts.Lines = defaultLines
	}
	if opts.Lines > maxLines {
		opts.Lines = maxLines
	}
	switch src.Kind {
	case KindFile:
		return readFile(src, opts)
	case KindPodman:
		return readPodman(src, opts)
	case KindJournal:
		return readJournal(src, opts) // platform-specific (read_linux.go / read_darwin.go)
	}
	return Result{}, fmt.Errorf("unsupported log source kind %d", src.Kind)
}

// fileScanCap bounds the entries parsed when a filter is active. The real bound
// is the byte tail ParseFile reads (applog.MaxReadBytes); this only has to be
// high enough never to drop an entry that fits in that window, so it is sized to
// the byte cap (one entry per byte being the theoretical maximum). Passing 0
// here instead would make ParseFile read the entire file uncapped, risking an
// OOM on a multi-hundred-MB log.
const fileScanCap = applog.MaxReadBytes

func readFile(src Source, opts Opts) (Result, error) {
	structured := src.Format == "monolog" || src.Format == "laravel"

	// When a filter that needs to look past the last N entries is active, scan
	// the whole capped tail. Level and time filters only apply to structured
	// formats (see below), so for a raw source they don't widen the scan — a bare
	// last-N read is correct there and avoids reading the whole tail for nothing.
	needScan := opts.Grep != "" || (structured && (opts.Level != "" || opts.Since != "" || opts.Until != ""))
	readCount := opts.Lines
	if needScan {
		readCount = fileScanCap
	}
	entries, err := applog.ParseFile(src.Locator, src.Format, readCount) // newest first
	if err != nil {
		return Result{}, err
	}

	matcher := compileGrep(opts.Grep)
	since, sinceOK := parseSince(opts.Since)
	until, untilOK := parseSince(opts.Until)

	kept := make([]Entry, 0, opts.Lines)
	for _, e := range entries { // newest -> oldest
		if opts.Level != "" && structured && !strings.EqualFold(e.Level, opts.Level) {
			continue
		}
		text := e.Message
		if e.Detail != "" {
			text = e.Detail
		}
		if matcher != nil && !matcher(text) {
			continue
		}
		if structured && (sinceOK || untilOK) {
			if t, ok := parseEntryTime(e.Date); ok {
				// since is exclusive so polling with a returned Cursor yields only
				// newer lines rather than re-emitting the boundary entry (and every
				// other entry sharing its whole-second timestamp) on each call.
				if sinceOK && !t.After(since) {
					continue
				}
				if untilOK && t.After(until) {
					continue
				}
			}
		}
		kept = append(kept, Entry{Time: e.Date, Level: e.Level, Channel: e.Channel, Text: text})
		if len(kept) >= opts.Lines {
			break
		}
	}

	reverse(kept) // chronological: oldest first
	res := Result{Entries: kept}
	if len(kept) > 0 {
		res.Cursor = kept[len(kept)-1].Time // newest
	}
	if info, err := os.Stat(src.Locator); err == nil && info.Size() > applog.MaxReadBytes {
		res.Truncated = true
	}
	return res, nil
}

// podmanScanCap bounds the tail we pull when filtering, so grep/time filters
// (applied in Go below) can see beyond the last opts.Lines lines without
// streaming the container's entire history. Mirrors readFile's whole-tail scan.
const podmanScanCap = 5000

func readPodman(src Source, opts Opts) (Result, error) {
	tail := opts.Lines
	if opts.Grep != "" || opts.Since != "" || opts.Until != "" {
		tail = podmanScanCap
	}
	args := []string{"logs", "--timestamps", "--tail", strconv.Itoa(tail)}
	if opts.Since != "" {
		args = append(args, "--since", podmanTime(opts.Since))
	}
	if opts.Until != "" {
		args = append(args, "--until", podmanTime(opts.Until))
	}
	args = append(args, src.Locator)

	var buf bytes.Buffer
	cmd := podman.Cmd(args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run() // non-zero when the container isn't running — return what we have

	matcher := compileGrep(opts.Grep)
	// `podman logs --since` is inclusive and only second-granular, and the cursor we
	// hand back is a full-precision timestamp, so polling with since=<cursor> would
	// otherwise re-emit the boundary line (and its same-second siblings) every call.
	// Drop entries at or before `since` here, mirroring the file path's exclusive
	// cursor so a poll yields only genuinely newer lines.
	since, sinceOK := parseSince(opts.Since)
	var out []Entry
	for _, line := range strings.Split(strings.TrimRight(StripANSI(buf.String()), "\n"), "\n") {
		if line == "" {
			continue
		}
		ts, text := splitPodmanTimestamp(line)
		if matcher != nil && !matcher(text) {
			continue
		}
		if sinceOK {
			if t, ok := parseAbs(ts); ok && !t.After(since) {
				continue
			}
		}
		out = append(out, Entry{Time: ts, Text: text})
	}
	// podman output is chronological; keep the newest opts.Lines after filtering.
	if len(out) > opts.Lines {
		out = out[len(out)-opts.Lines:]
	}
	cursor := ""
	if len(out) > 0 {
		cursor = out[len(out)-1].Time
	}
	return Result{Entries: out, Cursor: cursor}, nil
}

// ---- shared filter helpers ----

func compileGrep(pattern string) func(string) bool {
	if pattern == "" {
		return nil
	}
	if re, err := regexp.Compile(pattern); err == nil {
		return re.MatchString
	}
	return func(s string) bool { return strings.Contains(s, pattern) }
}

// parseSince accepts a Go relative duration ("15m", "2h30m"), an absolute
// timestamp (RFC3339, "2006-01-02 15:04:05", or "2006-01-02"), and reports
// whether it resolved to a usable time.
func parseSince(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d), true
	}
	return parseAbs(s)
}

// parseAbs parses an absolute timestamp. Zone-less forms are read as UTC to
// match monolog entry timestamps (Laravel's default app timezone), so a
// since/until value compares cleanly against the entries returned.
func parseAbs(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseEntryTime reads a monolog entry's zone-less timestamp as UTC, matching
// parseAbs so relative windows ("15m") and absolute bounds line up.
func parseEntryTime(s string) (time.Time, bool) {
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.UTC); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// podmanTime translates a since/until value into something `podman logs`
// understands: relative durations pass through, absolute times become RFC3339.
func podmanTime(s string) string {
	s = strings.TrimSpace(s)
	if _, err := time.ParseDuration(s); err == nil {
		return s
	}
	if t, ok := parseAbs(s); ok {
		return t.Format(time.RFC3339)
	}
	return s
}

func splitPodmanTimestamp(line string) (ts, text string) {
	if i := strings.IndexByte(line, ' '); i > 0 {
		cand := line[:i]
		if _, err := time.Parse(time.RFC3339Nano, cand); err == nil {
			return cand, line[i+1:]
		}
	}
	return "", line
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes ANSI escape sequences from log output.
func StripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func reverse(e []Entry) {
	for i, j := 0, len(e)-1; i < j; i, j = i+1, j-1 {
		e[i], e[j] = e[j], e[i]
	}
}
