package feedback

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTablePlainAligns(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)() // forces plain mode (no colour, no borders)

	Table(
		[]string{"Name", "Latest", "Versions"},
		[][]string{
			{"laravel", "13", "13, 12, 11"},
			{"symfony", "8", "8, 7"},
		},
	)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines (header + 2 rows), got %d: %q", len(lines), buf.String())
	}
	// Plain mode must not draw borders or ANSI escapes.
	if strings.ContainsAny(buf.String(), "│─╭╮╰╯\x1b") {
		t.Fatalf("plain table leaked borders/escapes: %q", buf.String())
	}
	// The "Latest" column header and the longer "Versions" value must start at
	// the same offset on every line, proving alignment.
	col := strings.Index(lines[0], "Latest")
	for i, ln := range lines[1:] {
		if got := strings.Index(ln, strings.Fields(ln)[1]); got != col {
			t.Errorf("row %d second column at %d, want %d: %q", i, got, col, ln)
		}
	}
}

func TestTableRaggedRowsNoPanic(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)()

	// A row shorter than the header must pad, not panic or drop columns.
	Table([]string{"A", "B", "C"}, [][]string{{"x"}, {"y", "z", "w"}})
	if !strings.Contains(buf.String(), "x") || !strings.Contains(buf.String(), "w") {
		t.Fatalf("ragged rows mangled: %q", buf.String())
	}
}

func TestRenderTableFitsTerminalWidth(t *testing.T) {
	mu.Lock()
	prevColor, prevWidth := colorOn.Load(), tableWidth
	colorOn.Store(true)
	tableWidth = func() int { return 60 }
	mu.Unlock()
	defer func() {
		mu.Lock()
		colorOn.Store(prevColor)
		tableWidth = prevWidth
		mu.Unlock()
	}()

	out := RenderTable(
		[]string{"Name", "Path"},
		[][]string{{"app", "/very/long/path/that/keeps/going/and/going/well/past/sixty/columns/wide"}},
	)
	for _, ln := range strings.Split(out, "\n") {
		if w := lipgloss.Width(ln); w > 60 {
			t.Errorf("line width %d exceeds budget 60: %q", w, ln)
		}
	}
	if !strings.Contains(out, "…") {
		t.Errorf("expected an ellipsis from truncating the long path: %q", out)
	}
}

func TestRenderTableColouredHasBorders(t *testing.T) {
	mu.Lock()
	prev := colorOn.Load()
	colorOn.Store(true)
	mu.Unlock()
	defer func() {
		mu.Lock()
		colorOn.Store(prev)
		mu.Unlock()
	}()

	out := RenderTable([]string{"Name"}, [][]string{{"laravel"}})
	if !strings.ContainsAny(out, "╭─│") {
		t.Fatalf("coloured table missing rounded border: %q", out)
	}
}

func TestFailPrintsCrossAndMarksShown(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)()

	err := errTest("boom")
	FailOn(&buf, err)

	out := buf.String()
	if !strings.Contains(out, "✗") || !strings.Contains(out, "boom") {
		t.Fatalf("Fail output missing cross or message: %q", out)
	}
	if !AlreadyShown(err) {
		t.Errorf("Fail should mark the error as already shown")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
