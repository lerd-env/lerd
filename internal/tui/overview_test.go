package tui

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/siteinfo"
)

func gridSite() *siteinfo.EnrichedSite {
	return &siteinfo.EnrichedSite{
		Name: "alpha", PHPVersion: "8.3", NodeVersion: "22",
		Domains:        []string{"alpha.test", "alt.test"},
		Services:       []string{"mysql"},
		HasQueueWorker: true,
		Worktrees: []siteinfo.WorktreeInfo{
			{Branch: "feat", Path: "/tmp/wt", PHPVersion: "8.3", NodeVersion: "22"},
		},
	}
}

// The cursor walks detailRows, so that order has to match the order the Overview
// draws its sections. When they drifted apart, `down` from a worker teleported the
// cursor to the Toggles block at the bottom of the pane, and `down` again threw it
// backwards into Worktrees.
func TestDetailRows_NavOrderMatchesRenderOrder(t *testing.T) {
	rows := detailRows(gridSite())
	nav := navigableRows(rows)

	var got []detailKind
	for _, i := range nav {
		got = append(got, rows[i].kind)
	}
	want := []detailKind{
		kindDomain, kindDomain, kindDomainAdd, // Domains
		kindPHP, kindNode, kindHTTPS, kindLANShare, // Toggles
		kindWorker,                      // Workers
		kindWorktreeDB, kindWorktreeLAN, // Worktrees
		kindWorktreePHP, kindWorktreeNode, //
	}
	if len(got) != len(want) {
		t.Fatalf("nav order\n got %v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("nav[%d] = %v, want %v\n got %v\nwant %v", i, got[i], want[i], got, want)
		}
	}
}

// The worktree header is a caption with no toggle, and it renders no cursor, so
// leaving it navigable made the cursor vanish for one keypress as it passed by.
func TestNavigableRows_SkipsTheWorktreeHeader(t *testing.T) {
	rows := detailRows(gridSite())
	for _, i := range navigableRows(rows) {
		if rows[i].kind == kindWorktreeHeader {
			t.Fatal("the worktree header should not be a cursor stop")
		}
	}
}

func TestOverviewCols_CollapsesBelowTheBreakpoint(t *testing.T) {
	if got := overviewCols(2*overviewMinColWidth + overviewGutter); got != 2 {
		t.Errorf("a pane exactly wide enough for two columns should use them, got %d", got)
	}
	if got := overviewCols(2*overviewMinColWidth + overviewGutter - 1); got != 1 {
		t.Errorf("one cell short of the breakpoint should collapse to one column, got %d", got)
	}
	if got := overviewColWidth(60); got != 60 {
		t.Errorf("a collapsed grid builds sections at the full pane width, got %d", got)
	}
}

func TestComposeOverview_PairsHalvesAndTracksTheCursor(t *testing.T) {
	left := ovSection{lines: []string{"L0", "L1", "L2"}, span: ovHalf, cursor: 2}
	right := ovSection{lines: []string{"R0"}, span: ovHalf, cursor: -1}
	full := ovSection{lines: []string{"F0"}, span: ovFull, cursor: -1}

	out, cursor := composeOverview([]ovSection{left, right, full}, 100)
	if len(out) != 4 {
		t.Fatalf("two paired sections plus a full one should be 4 rows, got %d:\n%v", len(out), out)
	}
	if !strings.Contains(out[0], "L0") || !strings.Contains(out[0], "R0") {
		t.Errorf("paired sections should share a row, got %q", out[0])
	}
	// The taller section keeps its rows; the shorter one just runs out.
	if !strings.Contains(out[2], "L2") {
		t.Errorf("the taller section should keep its remaining rows, got %q", out[2])
	}
	if cursor != 2 {
		t.Errorf("the cursor line should be offset into the composed block, got %d", cursor)
	}
	for i, ln := range out {
		if lipglossWidth(ln) != 100 {
			t.Errorf("row %d should span the pane, got width %d", i, lipglossWidth(ln))
		}
	}
}

func TestComposeOverview_NarrowStacksEverything(t *testing.T) {
	a := ovSection{lines: []string{"A"}, span: ovHalf, cursor: -1}
	b := ovSection{lines: []string{"B"}, span: ovHalf, cursor: -1}

	out, _ := composeOverview([]ovSection{a, b}, 40) // below the breakpoint
	if len(out) != 2 {
		t.Fatalf("a collapsed grid stacks its sections, got %d rows:\n%v", len(out), out)
	}
	if strings.Contains(out[0], "B") {
		t.Errorf("a collapsed grid must not pair sections, got %q", out[0])
	}
}

func TestHopDetailColumn_CrossesBetweenDomainsAndToggles(t *testing.T) {
	rows := detailRows(gridSite())
	nav := navigableRows(rows)
	wide := 2*overviewMinColWidth + overviewGutter

	// From the first domain, right lands on the first toggle.
	got := hopDetailColumn(rows, nav, 0, wide)
	if rows[nav[got]].kind != kindPHP {
		t.Fatalf("right from the first domain should reach the first toggle, got %v", rows[nav[got]].kind)
	}
	// The offset within the cell carries across, so the second domain reaches the
	// second toggle and back again.
	got = hopDetailColumn(rows, nav, 1, wide)
	if rows[nav[got]].kind != kindNode {
		t.Fatalf("the offset within the cell should carry across, got %v", rows[nav[got]].kind)
	}
	if back := hopDetailColumn(rows, nav, got, wide); back != 1 {
		t.Fatalf("hopping back should return to where it came from, got %d", back)
	}
}

func TestHopDetailColumn_ClampsAndNoOps(t *testing.T) {
	rows := detailRows(gridSite())
	nav := navigableRows(rows)
	wide := 2*overviewMinColWidth + overviewGutter

	// Domains has 3 rows, Toggles has 4: hopping from the last domain clamps into
	// the toggles rather than running off the end.
	got := hopDetailColumn(rows, nav, 2, wide)
	if k := rows[nav[got]].kind; k != kindHTTPS {
		t.Fatalf("the third domain should clamp onto the third toggle, got %v", k)
	}

	// A worker isn't part of the paired grid row, so there's nowhere to hop.
	worker := -1
	for i, idx := range nav {
		if rows[idx].kind == kindWorker {
			worker = i
			break
		}
	}
	if got := hopDetailColumn(rows, nav, worker, wide); got != worker {
		t.Errorf("a row outside the pair should not move, got %d want %d", got, worker)
	}

	// A one-column grid has no second column to reach.
	if got := hopDetailColumn(rows, nav, 0, 50); got != 0 {
		t.Errorf("a collapsed grid should not hop, got %d", got)
	}
}

func TestJoinInfo_PacksFactsUntilTheyStopFitting(t *testing.T) {
	got := joinInfo([]string{"aaa", "bbb", "ccc"}, 100)
	if len(got) != 1 {
		t.Fatalf("facts that fit should share one line, got %v", got)
	}
	got = joinInfo([]string{"aaa", "bbb", "ccc"}, 8)
	if len(got) < 2 {
		t.Fatalf("facts that don't fit should wrap, got %v", got)
	}
	if len(joinInfo([]string{"", ""}, 40)) != 0 {
		t.Error("empty facts should produce no lines")
	}
}

func TestTimingCols_DropsAColumnRatherThanStarveTheBlocks(t *testing.T) {
	if got := timingCols(3*timingMinBlockWidth + 2*overviewGutter); got != 3 {
		t.Errorf("a pane wide enough for three blocks should use three, got %d", got)
	}
	if got := timingCols(2*timingMinBlockWidth + overviewGutter); got != 2 {
		t.Errorf("expected two columns at the two-block width, got %d", got)
	}
	if got := timingCols(40); got != 1 {
		t.Errorf("a narrow pane stacks the blocks, got %d", got)
	}
}

// lipglossWidth is the display width of a rendered row.
func lipglossWidth(s string) int { return len([]rune(stripANSI(s))) }
