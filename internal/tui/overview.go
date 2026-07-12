package tui

import "github.com/charmbracelet/x/ansi"

// The site Overview lays out as a grid. A section either takes the whole pane
// width or pairs with the next half-width section, so Services sits beside
// Workers and Domains beside Toggles rather than stacking into a column twice as
// tall as the terminal. Below the breakpoint every section goes full-width and
// the grid degrades to the single column it has always been.
const (
	// overviewMinColWidth is the narrowest a paired column can get and still hold
	// a label, its state, and a glyph without truncating everything interesting.
	overviewMinColWidth = 44
	overviewGutter      = 2
)

// ovSpan says whether a section wants the whole pane width or shares a grid row.
type ovSpan int

const (
	ovHalf ovSpan = iota
	ovFull
)

// ovSection is one block of the Overview: the lines it renders, how wide it wants
// to be, and which of its lines carries the cursor (-1 when the cursor is
// elsewhere), so the grid can offset that line once the section lands in a row.
type ovSection struct {
	lines  []string
	span   ovSpan
	cursor int
}

// ovBuilder accumulates one section's lines at a fixed width, remembering the
// selected line as it goes.
type ovBuilder struct {
	lines  []string
	cursor int
	w      int
}

func newOvBuilder(w int) *ovBuilder { return &ovBuilder{cursor: -1, w: w} }

func (b *ovBuilder) add(s string, selected bool) {
	if selected {
		b.cursor = len(b.lines)
	}
	b.lines = append(b.lines, padToWidth(clipLine(s, b.w), b.w))
}

func (b *ovBuilder) plain(s string) { b.add(s, false) }

func (b *ovBuilder) empty() bool { return len(b.lines) == 0 }

// section freezes the builder into a section of the given span. Returns nothing
// for an empty builder, so a site with no worktrees doesn't leave a hole in the
// grid.
func (b *ovBuilder) section(span ovSpan) []ovSection {
	if b.empty() {
		return nil
	}
	return []ovSection{{lines: b.lines, span: span, cursor: b.cursor}}
}

// overviewCols reports how many columns the pane can carry. Two only when each
// would still clear overviewMinColWidth, so a narrow terminal keeps the single
// column rather than shredding both halves.
func overviewCols(innerW int) int {
	if (innerW-overviewGutter)/2 >= overviewMinColWidth {
		return 2
	}
	return 1
}

// overviewColWidth is the width a half-span section builds at: half the pane when
// the grid is two columns wide, the whole pane when it has collapsed to one.
func overviewColWidth(innerW int) int {
	if overviewCols(innerW) == 1 {
		return innerW
	}
	return (innerW - overviewGutter) / 2
}

// composeOverview lays the sections into the grid and returns the composed block
// plus the line carrying the cursor (-1 when no section holds it). Full-span
// sections take a row to themselves; a half-span section pairs with the next one,
// and falls back to full width when it has no partner or the grid is one column.
func composeOverview(secs []ovSection, innerW int) ([]string, int) {
	colW := overviewColWidth(innerW)
	paired := overviewCols(innerW) > 1

	var out []string
	cursorLine := -1
	// The right column takes whatever the split left over, so an odd pane width
	// still spans the full pane rather than leaving a ragged edge.
	rightW := innerW - colW - overviewGutter

	for i := 0; i < len(secs); i++ {
		left := secs[i]
		start := len(out)

		if paired && left.span == ovHalf && i+1 < len(secs) && secs[i+1].span == ovHalf {
			right := secs[i+1]
			h := len(left.lines)
			if len(right.lines) > h {
				h = len(right.lines)
			}
			for j := 0; j < h; j++ {
				l := spaces(colW)
				if j < len(left.lines) {
					l = padToWidth(clipLine(left.lines[j], colW), colW)
				}
				r := ""
				if j < len(right.lines) {
					r = clipLine(right.lines[j], rightW)
				}
				out = append(out, padToWidth(l+spaces(overviewGutter)+r, innerW))
			}
			if left.cursor >= 0 {
				cursorLine = start + left.cursor
			}
			if right.cursor >= 0 {
				cursorLine = start + right.cursor
			}
			i++ // the partner is consumed
			continue
		}

		for _, ln := range left.lines {
			out = append(out, padToWidth(clipLine(ln, innerW), innerW))
		}
		if left.cursor >= 0 {
			cursorLine = start + left.cursor
		}
	}
	return out, cursorLine
}

// detailInnerWidth is the content width the detail pane renders at, mirroring the
// split renderBody does. Key handlers need it to know whether the Overview grid
// is one column or two, since layout decides whether `left`/`right` mean anything.
func (m *Model) detailInnerWidth() int {
	w := m.width
	if m.width >= narrowWidth {
		leftW := m.width / 4
		leftW = clamp(leftW, 28, 46)
		if leftW > m.width-30 {
			leftW = m.width - 30
		}
		w = m.width - leftW
	}
	innerW, _ := innerSize(paneStyle(true), w, m.height)
	return innerW - 1 // the detail pane reserves a cell for the scrollbar
}

// ovCell names the grid cell a navigable row renders in. Only Domains and Toggles
// share a grid row with nav rows on both sides, so they're the only pair `left`
// and `right` can cross between.
type ovCell int

const (
	ovCellNone ovCell = iota
	ovCellDomains
	ovCellToggles
)

// ovCellOf classifies a row into its grid cell.
func ovCellOf(k detailKind) ovCell {
	switch k {
	case kindDomain, kindDomainAdd:
		return ovCellDomains
	case kindPHP, kindNode, kindHTTPS, kindLANShare:
		return ovCellToggles
	}
	return ovCellNone
}

// hopDetailColumn moves the cursor across the Domains/Toggles grid row, keeping
// its offset within the cell so `right` from the second domain lands on the
// second toggle. Returns the new cursor, or the old one when there's nowhere to
// hop: a one-column grid, or a row that isn't part of the pair.
func hopDetailColumn(rows []detailRow, nav []int, cursor int, innerW int) int {
	if overviewCols(innerW) == 1 || cursor < 0 || cursor >= len(nav) {
		return cursor
	}
	from := ovCellOf(rows[nav[cursor]].kind)
	if from == ovCellNone {
		return cursor
	}
	to := ovCellToggles
	if from == ovCellToggles {
		to = ovCellDomains
	}

	offset, first, count := 0, -1, 0
	for i, rowIdx := range nav {
		switch ovCellOf(rows[rowIdx].kind) {
		case from:
			if i < cursor {
				offset++
			}
		case to:
			if first < 0 {
				first = i
			}
			count++
		}
	}
	if first < 0 {
		return cursor
	}
	if offset >= count {
		offset = count - 1
	}
	return first + offset
}

// columnize lays blocks side by side, splitting innerW evenly between them and
// padding every row to the full width so the result spans the pane. Used inside a
// full-width section that wants to subdivide, like the timing panel's
// distribution / routes / recent trio.
func columnize(blocks [][]string, innerW int) []string {
	n := len(blocks)
	if n == 0 {
		return nil
	}
	colW := (innerW - overviewGutter*(n-1)) / n
	h := 0
	for _, b := range blocks {
		if len(b) > h {
			h = len(b)
		}
	}
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		row := ""
		for j, b := range blocks {
			cell := ""
			if i < len(b) {
				cell = b[i]
			}
			if j > 0 {
				row += spaces(overviewGutter)
			}
			row += padToWidth(clipLine(cell, colW), colW)
		}
		out = append(out, padToWidth(row, innerW))
	}
	return out
}

// joinInfo packs the identity facts onto as few lines as the width allows, so a
// wide pane doesn't spend five rows on one fact each.
func joinInfo(parts []string, w int) []string {
	var out []string
	cur := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		switch {
		case cur == "":
			cur = p
		case ansi.StringWidth(cur)+ansi.StringWidth(p)+3 <= w:
			cur += "   " + p
		default:
			out = append(out, cur)
			cur = p
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
