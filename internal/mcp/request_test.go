package mcp

import (
	"strings"
	"testing"
	"time"
)

const requestEventsBody = `[
	{"ts":"2026-10-07T10:00:00.000Z","kind":"browser","ctx":{"type":"browser","site":"acme","rid":"r1"},"data":{"type":"navigation"}},
	{"ts":"2026-10-07T10:00:00.010Z","kind":"query","ctx":{"type":"fpm","site":"acme","request":"GET /cart","rid":"r1"},"src":{"file":"/app/Cart.php","line":12},"data":{"sql":"select 1","time_ms":2,"trace":[{"file":"x"}]}},
	{"ts":"2026-10-07T10:00:00.020Z","kind":"query","ctx":{"type":"fpm","site":"acme","rid":"r1"},"data":{"sql":"select 2"}},
	{"ts":"2026-10-07T10:00:00.030Z","kind":"log","ctx":{"type":"fpm","site":"acme","rid":"r1"},"data":{"level":"error"}}
]`

// The lens bar an assistant reads names the request and counts each lens the
// way the dashboard does, leaving the page view out.
func TestRequestTool_LensesCountsEachLens(t *testing.T) {
	path := stubRoundTrip(t, requestEventsBody)
	got, _ := execRequestTool(map[string]any{"action": "lenses", "rid": "r1"})
	if *path != "/api/dumps?rid=r1" {
		t.Errorf("path = %q", *path)
	}
	text := toolText(got)
	for _, want := range []string{`"queries":2`, `"logs":1`, `"request":"GET /cart"`} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in %s", want, text)
		}
	}
	if strings.Contains(text, `"browser"`) {
		t.Errorf("a page view is not a browser event: %s", text)
	}
}

// What PHP reported as the request ended rides along with the counts, the way
// the dashboard shows it above the lenses, and is not a lens of its own.
func TestRequestTool_LensesCarryWhatPHPSpent(t *testing.T) {
	stubRoundTrip(t, `[
	{"ts":"2026-10-07T10:00:00.010Z","kind":"query","ctx":{"type":"fpm","site":"acme","rid":"r1"},"data":{"sql":"select 1","time_ms":2}},
	{"ts":"2026-10-07T10:00:00.090Z","kind":"request","ctx":{"type":"fpm","site":"acme","rid":"r1"},"data":{"time_ms":85.5,"memory_peak":4194304,"queue_ms":1.2,"trace":[{"file":"x"}]}}
]`)
	text := toolText(func() any { got, _ := execRequestTool(map[string]any{"action": "lenses", "rid": "r1"}); return got }())
	if !strings.Contains(text, `"php":{"memory_peak":4194304,"queue_ms":1.2,"time_ms":85.5}`) {
		t.Errorf("missing what PHP spent in %s", text)
	}
	if strings.Contains(text, "trace") || strings.Contains(text, `"request":1`) {
		t.Errorf("the summary must drop its trace and not count as a lens: %s", text)
	}
}

// One lens reads a page at a time, on the request's clock and without traces.
func TestRequestTool_LensPagesItsRows(t *testing.T) {
	path := stubRoundTrip(t, requestEventsBody)
	got, _ := execRequestTool(map[string]any{"action": "lens", "rid": "r1", "lens": "queries", "offset": 1, "limit": 1})
	if *path != "/api/dumps?rid=r1" {
		t.Errorf("path = %q", *path)
	}
	text := toolText(got)
	// The second query ran 20 ms after the request's first event, a page view.
	if !strings.Contains(text, `"total":2`) || !strings.Contains(text, `"n":2`) || !strings.Contains(text, `"offset_ms":20`) {
		t.Errorf("page = %s", text)
	}
	if strings.Contains(text, "trace") || strings.Contains(text, `"n":3`) {
		t.Errorf("page must drop traces and stop at the limit: %s", text)
	}
}

func TestRequestTool_NeedsARidAndALens(t *testing.T) {
	stubRoundTrip(t, `[]`)
	if got, _ := execRequestTool(map[string]any{"action": "lenses"}); !strings.Contains(toolText(got), "rid is required") {
		t.Errorf("no rid = %s", toolText(got))
	}
	if got, _ := execRequestTool(map[string]any{"action": "lens", "rid": "r1", "lens": "nope"}); !strings.Contains(toolText(got), "lens is required") {
		t.Errorf("unknown lens = %s", toolText(got))
	}
}

// The list holds the last hour's requests only, newest first, and a limit
// below one still returns a row rather than panicking.
func TestRecentInLastHour(t *testing.T) {
	now := time.UnixMilli(10_000_000)
	rows := []map[string]any{
		{"at_millis": float64(now.UnixMilli() - 1000)},
		{"at_millis": float64(now.UnixMilli() - 2000)},
		{"at_millis": float64(now.Add(-2 * time.Hour).UnixMilli())},
	}
	if got := recentInLastHour(rows, now, 20); len(got) != 2 {
		t.Errorf("last hour = %d rows, want 2", len(got))
	}
	if got := recentInLastHour(rows, now, 1); len(got) != 1 {
		t.Errorf("limit 1 = %d rows", len(got))
	}
}
