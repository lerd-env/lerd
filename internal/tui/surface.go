package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// surfaces are the background tints that separate regions instead of box
// borders: s1 is the sidebar, raised above the terminal background the content
// sits on, as in the web UI; s2..s4 are cards, selection and overlays. They are derived from the terminal's
// own background, so every theme gets matching tints without configuration.
type surfaces struct {
	bg, s1, s2, s3, s4 color.Color
}

var surf = untinted()

func untinted() surfaces {
	n := lipgloss.NoColor{}
	return surfaces{n, n, n, n, n}
}

// applySurfaces derives the tints from the terminal background bubbletea
// reported. An unknown background (nil) keeps every region untinted, which
// still reads correctly, just flatter.
func applySurfaces(bg color.Color) {
	if bg == nil {
		surf = untinted()
		return
	}
	r, g, b, _ := bg.RGBA()
	dark := (r>>8)*299+(g>>8)*587+(b>>8)*114 < 128000
	if dark {
		surf = surfaces{bg, mix(bg, color.White, 0.06), mix(bg, color.White, 0.045), mix(bg, color.White, 0.11), mix(bg, color.White, 0.16)}
		return
	}
	surf = surfaces{bg, mix(bg, color.White, 0.6), mix(bg, color.Black, 0.04), mix(bg, color.Black, 0.08), mix(bg, color.Black, 0.12)}
}

func mix(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	ch := func(x, y uint32) uint8 { return uint8(float64(x>>8)*(1-t) + float64(y>>8)*t) }
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", ch(ar, br), ch(ag, bg), ch(ab, bb)))
}

// layout is how the screen splits at a given terminal size.
type layout struct {
	sideW   int  // sidebar width, 0 when it folds away
	padX    int  // main-area margin on each side
	compact bool // short terminal: optional spacing rows are dropped
}

func layoutFor(w, h int) layout {
	l := layout{compact: h < 40}
	switch {
	case w >= 130:
		l.sideW, l.padX = 34, 4
	case w >= 96:
		l.sideW, l.padX = 28, 3
	default:
		l.sideW, l.padX = 0, 2
	}
	return l
}

// seg is one run of text in a row: its colour and weight on the row's surface.
type seg struct {
	t    string
	fg   color.Color
	bold bool
	bg   color.Color // overrides the row's surface, for a pill inside a row
}

func sp(t string, fg color.Color) seg { return seg{t: t, fg: fg} }
func bd(t string, fg color.Color) seg { return seg{t: t, fg: fg, bold: true} }

// row paints segs onto one surface and pads or cuts it to exactly w cells.
// Every seg carries the surface itself, because an inner style reset would
// otherwise punch a hole in the tint.
func row(bg color.Color, w int, segs ...seg) string {
	if bg == nil {
		bg = lipgloss.NoColor{}
	}
	var b strings.Builder
	n := 0
	for _, g := range segs {
		fg := g.fg
		if fg == nil {
			fg = lipgloss.NoColor{}
		}
		segBg := bg
		if g.bg != nil {
			segBg = g.bg
		}
		b.WriteString(lipgloss.NewStyle().Foreground(fg).Background(segBg).Bold(g.bold).Render(g.t))
		n += ansi.StringWidth(g.t)
	}
	if n > w {
		return ansi.Truncate(b.String(), w, "")
	}
	return b.String() + lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", w-n))
}

// rowLR right-aligns the second group. When both do not fit, the left text is
// ellipsized; when that would leave it unreadable, the right group goes.
func rowLR(bg color.Color, w int, left, right []seg) string {
	lw, rw := segsWidth(left), segsWidth(right)
	if lw+rw+1 > w {
		if w-rw-1 < 12 {
			return row(bg, w, left...)
		}
		left = ellipsize(left, w-rw-2)
		lw = segsWidth(left)
	}
	all := append(append(append([]seg{}, left...), sp(strings.Repeat(" ", max(1, w-lw-rw)), nil)), right...)
	return row(bg, w, all...)
}

func segsWidth(segs []seg) int {
	n := 0
	for _, g := range segs {
		n += ansi.StringWidth(g.t)
	}
	return n
}

// ellipsize cuts segs to room cells, ending on "…".
func ellipsize(segs []seg, room int) []seg {
	var out []seg
	for _, g := range segs {
		gw := ansi.StringWidth(g.t)
		if gw > room {
			g.t = ansi.Truncate(g.t, max(0, room), "…")
			return append(out, g)
		}
		out, room = append(out, g), room-gw
	}
	return out
}

// splice overwrites base from column x with over, keeping the rest of the line.
// Overlays (palette, sidebar on narrow terminals, toasts) are drawn with it.
func splice(base, over string, x int) string {
	return ansi.Truncate(base, x, "") + over + ansi.TruncateLeft(base, x+ansi.StringWidth(over), "")
}
