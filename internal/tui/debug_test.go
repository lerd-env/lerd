package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

func setLens(m *Model, kind string) {
	for i, l := range debugLenses {
		if l.kind == kind {
			m.debugLens = i
			return
		}
	}
}

func qEv(id, rid, sql string, ms float64) dumps.Event {
	data, _ := json.Marshal(map[string]any{"sql": sql, "time_ms": ms})
	return dumps.Event{
		ID:   id,
		TS:   "2026-05-10T00:00:0" + id + ".000Z",
		Kind: dumps.KindQuery,
		Ctx:  dumps.Context{Type: "fpm", Site: "acme", Request: "GET /", RID: rid},
		Data: data,
	}
}

func evWithData(kind string, fields map[string]any) dumps.Event {
	data, _ := json.Marshal(fields)
	return dumps.Event{ID: "e", TS: "2026-05-10T00:00:00.000Z", Kind: kind, Ctx: dumps.Context{Type: "fpm", Site: "acme", RID: "r"}, Data: data}
}

func TestDebugGroups_FlagsNPlusOneForRepeatedShape(t *testing.T) {
	m := NewModel("test")
	setLens(m, dumps.KindQuery)
	// Three structurally-identical queries (literals differ) in one request.
	for i := 1; i <= 3; i++ {
		m.appendDebug(qEv(fmt.Sprint(i), "r1", fmt.Sprintf("select * from users where id = %d", i), 2))
	}
	groups := m.debugGroups("")
	if len(groups) != 1 {
		t.Fatalf("expected 1 request group, got %d", len(groups))
	}
	if !groups[0].nPlusOne {
		t.Error("three same-shape queries in one request should flag N+1")
	}
}

func TestDebugGroups_DistinctQueriesAreNotNPlusOne(t *testing.T) {
	m := NewModel("test")
	setLens(m, dumps.KindQuery)
	m.appendDebug(qEv("1", "r1", "select * from users", 2))
	m.appendDebug(qEv("2", "r1", "select * from posts", 2))
	groups := m.debugGroups("")
	if len(groups) != 1 || groups[0].nPlusOne {
		t.Errorf("distinct queries should not flag N+1: groups=%d", len(groups))
	}
}

func TestDebugGroups_SeparateRequestsStaySeparate(t *testing.T) {
	m := NewModel("test")
	setLens(m, dumps.KindQuery)
	m.appendDebug(qEv("1", "r1", "select 1", 1))
	m.appendDebug(qEv("2", "r2", "select 2", 1))
	if got := len(m.debugGroups("")); got != 2 {
		t.Errorf("two request ids should yield two groups, got %d", got)
	}
}

func TestDebugVisibleEvents_RestrictsToActiveLens(t *testing.T) {
	m := NewModel("test")
	m.appendDebug(dumpEv(DumpEntry{ID: "d", Text: "x"}))
	m.appendDebug(qEv("1", "r1", "select 1", 1))
	setLens(m, dumps.KindQuery)
	vis := m.debugVisibleEvents("")
	if len(vis) != 1 || vis[0].Kind != dumps.KindQuery {
		t.Errorf("query lens should show only query events, got %+v", vis)
	}
	setLens(m, dumps.KindDump)
	if vis := m.debugVisibleEvents(""); len(vis) != 1 || vis[0].Kind != dumps.KindDump {
		t.Errorf("dump lens should show only dump events, got %+v", vis)
	}
}

func TestCycleDebugLens_WrapsAndResetsCursor(t *testing.T) {
	m := NewModel("test")
	m.dumpsCursor = 5
	m.cycleDebugLens(-1)
	if m.debugLens != len(debugLenses)-1 {
		t.Errorf("cycling back from 0 should wrap to last lens, got %d", m.debugLens)
	}
	if m.dumpsCursor != 0 {
		t.Errorf("lens switch should reset cursor, got %d", m.dumpsCursor)
	}
	m.cycleDebugLens(1)
	if m.debugLens != 0 {
		t.Errorf("cycling forward should wrap back to 0, got %d", m.debugLens)
	}
}

func TestDebugContentLines_QueryLensShowsSQLSlowAndTabs(t *testing.T) {
	m := NewModel("test")
	setLens(m, dumps.KindQuery)
	m.appendDebug(qEv("1", "r1", "select * from orders", 250))
	joined := stripANSI(strings.Join(firstReturn(debugContentLines(m, true, 120)), "\n"))
	if !strings.Contains(joined, "Queries") {
		t.Errorf("lens tab bar should list Queries:\n%s", joined)
	}
	if !strings.Contains(joined, "select * from orders") {
		t.Errorf("query SQL should render:\n%s", joined)
	}
	if !strings.Contains(joined, "slow") {
		t.Errorf("a 250ms query should be tagged slow:\n%s", joined)
	}
}

func TestDebugRowMain_RendersPerKindFields(t *testing.T) {
	job := evWithData(dumps.KindJob, map[string]any{"class": "App\\Jobs\\Send", "status": "failed"})
	if got := stripANSI(debugRowMain(dumps.KindJob, job, nil)); !strings.Contains(got, "App\\Jobs\\Send") || !strings.Contains(got, "failed") {
		t.Errorf("job row: %q", got)
	}
	mail := evWithData(dumps.KindMail, map[string]any{"subject": "Welcome", "to": []string{"a@x.com"}})
	if got := stripANSI(debugRowMain(dumps.KindMail, mail, nil)); !strings.Contains(got, "Welcome") || !strings.Contains(got, "a@x.com") {
		t.Errorf("mail row: %q", got)
	}
	httpEv := evWithData(dumps.KindHTTP, map[string]any{"method": "GET", "url": "https://api/x", "status": 200})
	if got := stripANSI(debugRowMain(dumps.KindHTTP, httpEv, nil)); !strings.Contains(got, "GET https://api/x") || !strings.Contains(got, "200") {
		t.Errorf("http row: %q", got)
	}
	cache := evWithData(dumps.KindCache, map[string]any{"key": "user:1", "op": "hit"})
	if got := stripANSI(debugRowMain(dumps.KindCache, cache, nil)); !strings.Contains(got, "user:1") || !strings.Contains(got, "hit") {
		t.Errorf("cache row: %q", got)
	}
}

func TestDebugBatchMsg_AppendsAllEvents(t *testing.T) {
	m := NewModel("test")
	batch := debugBatchMsg{
		dumpEv(DumpEntry{ID: "a", Text: "x"}),
		qEv("1", "r1", "select 1", 1),
		dumpEv(DumpEntry{ID: "a", Text: "dup"}), // de-duped on ID
	}
	if _, cmd := m.Update(batch); cmd != nil {
		t.Errorf("debugBatchMsg should not emit a command, got %v", cmd)
	}
	if len(m.debug) != 2 {
		t.Errorf("expected 2 events after batch dedup, got %d", len(m.debug))
	}
}

func TestBracketKey_SwitchesLensInDebugView(t *testing.T) {
	m := NewModel("test")
	m.detailMode = detailDumps
	m.focus = paneDetail
	if m.debugLens != 0 {
		t.Fatalf("expected to start on the Dumps lens, got %d", m.debugLens)
	}
	// `]` advances to the next lens even with the log pane irrelevant here.
	if _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")}); m.debugLens != 1 {
		t.Errorf("] should advance to lens 1 (Queries), got %d", m.debugLens)
	}
	if _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")}); m.debugLens != 0 {
		t.Errorf("[ should move back to lens 0 (Dumps), got %d", m.debugLens)
	}
}

func TestToggleDebugWorkers_PersistsState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := NewModel("test")
	m.toggleDebugWorkers()
	if cfg, _ := config.LoadGlobal(); cfg == nil || !cfg.IsDevtoolsWorkers() {
		t.Fatal("worker capture should be on after first toggle")
	}
	m.toggleDebugWorkers()
	if cfg, _ := config.LoadGlobal(); cfg != nil && cfg.IsDevtoolsWorkers() {
		t.Error("worker capture should be off after second toggle")
	}
}

// firstReturn drops the cursor-line second return so a test can inline a
// ([]string, int) call into strings.Join.
func firstReturn(lines []string, _ int) []string { return lines }
