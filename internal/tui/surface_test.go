package tui

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestLayoutForBreakpoints(t *testing.T) {
	cases := []struct {
		w, h        int
		side, pad   int
		compact     bool
		description string
	}{
		{160, 50, 34, 4, false, "wide gets the full sidebar"},
		{130, 50, 34, 4, false, "130 is still wide"},
		{129, 50, 28, 3, false, "medium gets the slim sidebar"},
		{96, 50, 28, 3, false, "96 is still medium"},
		{95, 50, 0, 2, false, "narrow folds the sidebar away"},
		{160, 39, 34, 4, true, "short terminals go compact"},
		{160, 40, 34, 4, false, "40 rows is roomy"},
	}
	for _, c := range cases {
		l := layoutFor(c.w, c.h)
		if l.sideW != c.side || l.padX != c.pad || l.compact != c.compact {
			t.Errorf("%s: layoutFor(%d,%d) = %+v", c.description, c.w, c.h, l)
		}
	}
}

func TestRowPadsToExactWidth(t *testing.T) {
	got := row(nil, 20, sp("abc", colDim))
	if w := ansi.StringWidth(got); w != 20 {
		t.Fatalf("row width = %d, want 20", w)
	}
}

func TestRowTruncatesOverflow(t *testing.T) {
	got := row(nil, 5, sp("abcdefgh", colDim))
	if w := ansi.StringWidth(got); w != 5 {
		t.Fatalf("row width = %d, want 5", w)
	}
	if ansi.Strip(got) != "abcde" {
		t.Fatalf("row text = %q", ansi.Strip(got))
	}
}

func TestRowLRRightAligns(t *testing.T) {
	got := ansi.Strip(rowLR(nil, 12, []seg{sp("ab", colDim)}, []seg{sp("xy", colDim)}))
	if got != "ab        xy" {
		t.Fatalf("rowLR = %q", got)
	}
}

// On overflow the right group (a key hint, a status) is what the eye looks for,
// so it survives and the left text takes the ellipsis.
func TestRowLRKeepsRightGroupOnOverflow(t *testing.T) {
	got := ansi.Strip(rowLR(nil, 20, []seg{sp("a-very-long-site-name.test", colDim)}, []seg{sp("running", colDim)}))
	if ansi.StringWidth(got) != 20 {
		t.Fatalf("width = %d, want 20: %q", ansi.StringWidth(got), got)
	}
	if got[len(got)-len("running"):] != "running" {
		t.Fatalf("right group lost: %q", got)
	}
}

func TestRowLRDropsRightGroupWhenLeftWouldBeUnreadable(t *testing.T) {
	got := ansi.Strip(rowLR(nil, 14, []seg{sp("shop.test", colDim)}, []seg{sp("enter open", colDim)}))
	if got != "shop.test     " {
		t.Fatalf("rowLR = %q", got)
	}
}

func TestSpliceOverwritesInPlace(t *testing.T) {
	base := row(nil, 10, sp("0123456789", colDim))
	got := ansi.Strip(splice(base, "ab", 3))
	if got != "012ab56789" {
		t.Fatalf("splice = %q", got)
	}
}

func TestApplySurfacesUnknownBackgroundLeavesNoTint(t *testing.T) {
	defer applySurfaces(nil)
	applySurfaces(nil)
	for i, c := range []color.Color{surf.bg, surf.s1, surf.s2, surf.s3, surf.s4} {
		if _, ok := c.(lipgloss.NoColor); !ok {
			t.Fatalf("surface %d = %v, want NoColor", i, c)
		}
	}
}

func TestApplySurfacesRaisesTowardForeground(t *testing.T) {
	defer applySurfaces(nil)
	applySurfaces(color.RGBA{0x1a, 0x1b, 0x26, 0xff})
	lum := func(c color.Color) uint32 { r, g, b, _ := c.RGBA(); return r + g + b }
	// The sidebar sits above the content, and its selection above the sidebar.
	if !(lum(surf.bg) < lum(surf.s2) && lum(surf.s2) < lum(surf.s1) && lum(surf.s1) < lum(surf.s3) && lum(surf.s3) < lum(surf.s4)) {
		t.Fatalf("dark surfaces not ordered: bg %v s2 %v s1 %v s3 %v s4 %v", surf.bg, surf.s2, surf.s1, surf.s3, surf.s4)
	}

	applySurfaces(color.RGBA{0xef, 0xf1, 0xf5, 0xff})
	if !(lum(surf.s2) < lum(surf.bg) && lum(surf.s3) < lum(surf.s2)) {
		t.Fatalf("light surfaces should darken: bg %v s2 %v s3 %v", surf.bg, surf.s2, surf.s3)
	}
}
