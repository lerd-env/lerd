package logsource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const monologFixture = `[2026-06-11 10:00:00] local.INFO: started up
[2026-06-11 10:05:00] local.ERROR: SQLSTATE[HY000] connection refused
[2026-06-11 10:10:00] local.WARNING: slow query detected
[2026-06-11 10:15:00] local.ERROR: SQLSTATE[42S02] table missing
`

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "laravel.log")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func monologSource(t *testing.T, content string) Source {
	return Source{Name: "app:laravel.log", Kind: KindFile, Locator: writeFixture(t, content), Format: "monolog"}
}

func TestRead_File_NoFilter_Chronological(t *testing.T) {
	res, err := Read(monologSource(t, monologFixture), Opts{Lines: 10})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Entries) != 4 {
		t.Fatalf("want 4 entries, got %d", len(res.Entries))
	}
	if !contains(res.Entries[0].Text, "started up") {
		t.Errorf("first entry should be oldest, got %q", res.Entries[0].Text)
	}
	if !contains(res.Entries[3].Text, "table missing") {
		t.Errorf("last entry should be newest, got %q", res.Entries[3].Text)
	}
	if res.Cursor != "2026-06-11 10:15:00" {
		t.Errorf("cursor = %q, want newest entry date", res.Cursor)
	}
}

func TestRead_File_Grep(t *testing.T) {
	cases := []struct {
		name string
		grep string
		want int
	}{
		{"literal-substring-as-regex", "SQLSTATE", 2},
		{"regex", "42S0[0-9]", 1},
		{"invalid-regex-falls-back-to-literal", "[", 2},
		{"no-match", "nope", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Read(monologSource(t, monologFixture), Opts{Lines: 10, Grep: tc.grep})
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if len(res.Entries) != tc.want {
				t.Fatalf("grep %q: want %d entries, got %d", tc.grep, tc.want, len(res.Entries))
			}
		})
	}
}

func TestRead_File_Level(t *testing.T) {
	res, err := Read(monologSource(t, monologFixture), Opts{Lines: 10, Level: "error"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("want 2 error entries, got %d", len(res.Entries))
	}
	for _, e := range res.Entries {
		if e.Level != "ERROR" {
			t.Errorf("entry level = %q, want ERROR", e.Level)
		}
	}
}

func TestRead_File_LinesCap(t *testing.T) {
	res, err := Read(monologSource(t, monologFixture), Opts{Lines: 2})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(res.Entries))
	}
	// Newest two, still chronological.
	if !contains(res.Entries[0].Text, "slow query") || !contains(res.Entries[1].Text, "table missing") {
		t.Errorf("unexpected window: %q / %q", res.Entries[0].Text, res.Entries[1].Text)
	}
}

// TestRead_LinesClampedToMax guards the MCP-exposed lines arg: a huge value must
// be clamped before it reaches make([]Entry, 0, n), which would otherwise OOM the
// process on an unrecoverable fatal error rather than just return fewer lines.
func TestRead_LinesClampedToMax(t *testing.T) {
	res, err := Read(monologSource(t, monologFixture), Opts{Lines: 1 << 40})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// The fixture has only a handful of entries; the point is that the absurd Lines
	// value was clamped (no OOM) and we still got the available window back.
	if len(res.Entries) == 0 {
		t.Fatalf("want the available entries back, got none")
	}
}

func TestRead_File_TimeWindow(t *testing.T) {
	res, err := Read(monologSource(t, monologFixture), Opts{Lines: 10, Since: "2026-06-11 10:07:00"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("want 2 entries after 10:07, got %d", len(res.Entries))
	}
	if !contains(res.Entries[0].Text, "slow query") {
		t.Errorf("first kept entry = %q", res.Entries[0].Text)
	}
}

func TestRead_File_CursorAdvances(t *testing.T) {
	src := monologSource(t, monologFixture)
	first, err := Read(src, Opts{Lines: 10})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	second, err := Read(src, Opts{Lines: 10, Since: first.Cursor})
	if err != nil {
		t.Fatalf("Read poll: %v", err)
	}
	// No new lines were written, so polling with the cursor returns nothing — the
	// boundary entry is not re-emitted (since is exclusive of the cursor instant).
	if len(second.Entries) != 0 {
		t.Fatalf("polling with cursor and no new lines should return 0 entries, got %d: %+v", len(second.Entries), second.Entries)
	}

	// A line newer than the cursor is delivered; an appended same-second sibling
	// of an already-seen line is not (the cursor only resolves to the second).
	appended := monologFixture + "[2026-06-11 10:20:00] local.INFO: fresh line\n"
	if err := os.WriteFile(src.Locator, []byte(appended), 0644); err != nil {
		t.Fatalf("append fixture: %v", err)
	}
	third, err := Read(src, Opts{Lines: 10, Since: first.Cursor})
	if err != nil {
		t.Fatalf("Read poll after append: %v", err)
	}
	if len(third.Entries) != 1 || !contains(third.Entries[0].Text, "fresh line") {
		t.Fatalf("polling after a new line should return just that line, got %d: %+v", len(third.Entries), third.Entries)
	}
}

// A large structured log with a grep filter must not be read in full: ParseFile
// is byte-capped at applog.MaxReadBytes, and the filtered path must keep that cap
// rather than reading the whole file into memory.
func TestRead_File_FilteredReadIsByteCapped(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40000; i++ { // well over MaxReadBytes once rendered
		b.WriteString("[2026-06-11 10:00:00] local.INFO: noise line padding padding padding padding\n")
	}
	b.WriteString("[2026-06-11 23:59:59] local.ERROR: needle in the recent tail\n")
	src := monologSource(t, b.String())

	res, err := Read(src, Opts{Lines: 10, Grep: "needle"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !res.Truncated {
		t.Errorf("a file larger than the byte cap should report Truncated")
	}
	if len(res.Entries) != 1 || !contains(res.Entries[0].Text, "needle") {
		t.Fatalf("grep should find the needle in the capped tail, got %d: %+v", len(res.Entries), res.Entries)
	}
}

// A level filter on a raw (non-structured) source can't apply, so it must not be
// silently ignored by reading the whole file — it falls back to a plain last-N.
func TestRead_RawFile_LevelIsBestEffortNoOp(t *testing.T) {
	raw := "line one\nline two\nline three\n"
	src := Source{Name: "raw", Kind: KindFile, Locator: writeFixture(t, raw), Format: "raw"}
	res, err := Read(src, Opts{Lines: 2, Level: "error"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("raw level filter should be a last-N no-op, got %d entries", len(res.Entries))
	}
}

func TestRead_RawFile_SinceIsBestEffortNoOp(t *testing.T) {
	raw := "line one\nline two\nline three\n"
	src := Source{Name: "raw", Kind: KindFile, Locator: writeFixture(t, raw), Format: "raw"}
	res, err := Read(src, Opts{Lines: 10, Since: "5m"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Entries) != 3 {
		t.Fatalf("raw since should be a no-op (last-N), got %d entries", len(res.Entries))
	}
	if res.Cursor != "" {
		t.Errorf("raw source has no timestamp cursor, got %q", res.Cursor)
	}
}

func TestParseSince(t *testing.T) {
	now := time.Now()
	cases := []struct {
		in   string
		ok   bool
		near time.Time
	}{
		{"15m", true, now.Add(-15 * time.Minute)},
		{"2h30m", true, now.Add(-150 * time.Minute)},
		{"2026-06-11T10:00:00Z", true, time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)},
		{"", false, time.Time{}},
		{"garbage", false, time.Time{}},
	}
	for _, tc := range cases {
		got, ok := parseSince(tc.in)
		if ok != tc.ok {
			t.Errorf("parseSince(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if tc.ok && got.Sub(tc.near).Abs() > 2*time.Second {
			t.Errorf("parseSince(%q) = %v, want near %v", tc.in, got, tc.near)
		}
	}
}

func TestCompileGrep(t *testing.T) {
	if compileGrep("") != nil {
		t.Error("empty pattern should yield no matcher")
	}
	re := compileGrep("ab.c")
	if !re("abXc") || re("abc") {
		t.Error("regex matcher behaved unexpectedly")
	}
	lit := compileGrep("[") // invalid regex
	if !lit("a[b") || lit("ab") {
		t.Error("literal fallback behaved unexpectedly")
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
