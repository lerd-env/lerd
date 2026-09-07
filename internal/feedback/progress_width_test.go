package feedback

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A progress line wider than the terminal wraps onto a second row. The repaint
// only returns to and clears the row the cursor is on, so the wrapped part is
// left behind and every frame adds another line instead of replacing one.
func TestProgressDrawFitsTheTerminalWidth(t *testing.T) {
	var buf bytes.Buffer
	prevOut, prevWidth, prevColor := out, tableWidth, colorOn.Load()
	out = &buf
	tableWidth = func() int { return 40 }
	colorOn.Store(true)
	t.Cleanup(func() { out, tableWidth = prevOut, prevWidth; colorOn.Store(prevColor) })

	p := &Progress{msg: "refreshing 40 store definitions", total: 40, animated: true}
	p.done.Store(24)
	p.current = "cakephp/migrations-and-a-very-long-package-name"
	p.draw("|")

	for _, line := range strings.Split(buf.String(), "\n") {
		if w := ansi.StringWidth(stripControl(line)); w >= 40 {
			t.Errorf("drew %d visible columns into a 40 column terminal, so it wraps:\n%q", w, line)
		}
	}
	if strings.Count(buf.String(), "\n") != 0 {
		t.Errorf("a repaint must not emit a newline:\n%q", buf.String())
	}
}

// An unknown width (piped or redirected) has nothing to fit into, so the line
// is left whole rather than cut to an invented number.
func TestProgressDrawLeavesAnUnknownWidthAlone(t *testing.T) {
	var buf bytes.Buffer
	prevOut, prevWidth, prevColor := out, tableWidth, colorOn.Load()
	out = &buf
	tableWidth = func() int { return 0 }
	colorOn.Store(true)
	t.Cleanup(func() { out, tableWidth = prevOut, prevWidth; colorOn.Store(prevColor) })

	p := &Progress{msg: "refreshing", total: 40, animated: true}
	p.current = "a-package-name-long-enough-to-pass-any-sane-terminal-width-limit"
	p.draw("")

	if !strings.Contains(buf.String(), "any-sane-terminal-width-limit") {
		t.Errorf("an unknown width must not truncate:\n%q", buf.String())
	}
}

// stripControl drops the cursor moves the repaint prefixes so only the drawn
// text is measured.
func stripControl(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	return strings.ReplaceAll(s, "\033[2K", "")
}
