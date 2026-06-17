//go:build linux

package logsource

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// readJournal reads a unit's logs from the systemd user journal. Time and grep
// filters push down to journalctl; the cursor is the journal's own opaque
// __CURSOR so the next poll resumes exactly with --after-cursor.
func readJournal(src Source, opts Opts) (Result, error) {
	args, fallback := journalArgs(src, opts)
	tailCap := journalTailCap(opts)

	var buf bytes.Buffer
	cmd := exec.Command("journalctl", args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run() // missing unit / no journal access — return what we have

	var out []Entry
	var cursor string
	raw := 0
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for {
		var je journalEntry
		if err := dec.Decode(&je); err != nil {
			break
		}
		raw++
		// Advance the cursor past every decoded entry, including ones the literal
		// fallback drops, so the next poll resumes after them rather than re-scanning.
		if je.Cursor != "" {
			cursor = je.Cursor
		}
		text := je.message()
		if fallback != nil && !fallback(text) {
			continue
		}
		out = append(out, Entry{Time: je.timeString(), Text: text})
	}
	// journalctl -n tails the newest cap entries; if it returned a full cap's
	// worth, older lines were dropped, so report it truncated like the file and
	// podman paths rather than letting the loss pass silently.
	truncated := tailCap > 0 && raw >= tailCap
	return Result{Entries: out, Cursor: cursor, Truncated: truncated}, nil
}

// journalTailCap is the -n value journalArgs applies: the per-call tail on a
// fresh read, or maxLines when resuming from a cursor.
func journalTailCap(opts Opts) int {
	if opts.Since != "" && isJournalCursor(opts.Since) {
		return maxLines
	}
	return opts.Lines
}

// journalArgs builds the journalctl argv for a unit read and returns an
// in-process grep fallback when the pattern can't be pushed down. The -n tail
// (journalTailCap) is the per-call opts.Lines on a fresh read, raised to maxLines
// on a cursor resume so a busy unit's recent lines aren't clipped to the small
// per-call tail while memory stays bounded; readJournal flags Truncated when the
// cap is hit so the dropped older lines aren't a silent loss.
func journalArgs(src Source, opts Opts) ([]string, func(string) bool) {
	args := []string{"--user", "-u", src.Locator + ".service", "--no-pager", "--output=json"}
	resuming := opts.Since != "" && isJournalCursor(opts.Since)
	if opts.Since != "" {
		if resuming {
			args = append(args, "--after-cursor="+opts.Since)
		} else {
			args = append(args, "--since", journalTime(opts.Since))
		}
	}
	if opts.Until != "" {
		args = append(args, "--until", journalTime(opts.Until))
	}
	// journalctl -g takes a regex; an invalid one makes it error out and we'd
	// silently return nothing. When the pattern won't compile, drop the push-down
	// and filter in-process with the shared literal fallback, matching the file
	// and podman paths so the same grep input behaves the same on every source.
	var fallback func(string) bool
	if opts.Grep != "" {
		if _, err := regexp.Compile(opts.Grep); err == nil {
			args = append(args, "-g", opts.Grep)
		} else {
			fallback = compileGrep(opts.Grep)
		}
	}
	if tailCap := journalTailCap(opts); tailCap > 0 {
		args = append(args, "-n", strconv.Itoa(tailCap))
	}
	return args, fallback
}

func isJournalCursor(s string) bool {
	return strings.HasPrefix(s, "s=") && strings.Contains(s, ";i=")
}

// journalTime converts a since/until value to journalctl's accepted forms:
// relative Go durations become "-15m"; absolute times become "YYYY-MM-DD HH:MM:SS".
func journalTime(s string) string {
	s = strings.TrimSpace(s)
	if _, err := time.ParseDuration(s); err == nil {
		// "-" makes it a look-back; strip any sign the user already typed so a
		// "-15m" typo becomes "-15m" not the invalid "--15m".
		return "-" + strings.TrimLeft(s, "+-")
	}
	if t, ok := parseAbs(s); ok {
		// journalctl reads a zone-less time in the system's local zone, so emit
		// the local wall-clock of the parsed instant.
		return t.In(time.Local).Format("2006-01-02 15:04:05")
	}
	return s
}

type journalEntry struct {
	Cursor   string          `json:"__CURSOR"`
	Realtime string          `json:"__REALTIME_TIMESTAMP"`
	Message  json.RawMessage `json:"MESSAGE"`
}

func (j journalEntry) message() string {
	if len(j.Message) == 0 {
		return ""
	}
	if j.Message[0] == '"' {
		var s string
		_ = json.Unmarshal(j.Message, &s)
		return s
	}
	// journalctl encodes binary messages as a JSON array of bytes.
	var b []byte
	if json.Unmarshal(j.Message, &b) == nil {
		return string(b)
	}
	return ""
}

func (j journalEntry) timeString() string {
	if j.Realtime == "" {
		return ""
	}
	usec, err := strconv.ParseInt(j.Realtime, 10, 64)
	if err != nil {
		return ""
	}
	return time.Unix(0, usec*1000).Format(time.RFC3339)
}
