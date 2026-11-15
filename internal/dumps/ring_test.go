package dumps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mkEvent(id string) Event {
	return Event{V: 1, ID: id, Kind: KindDump, Ctx: Context{Type: "fpm", Site: "acme"}}
}

func TestRing_AppendUnderCap(t *testing.T) {
	r := NewRing(4)
	r.Append(mkEvent("a"))
	r.Append(mkEvent("b"))
	got := r.Snapshot()
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("snapshot = %v", ids(got))
	}
}

func TestRing_AppendWrapsAroundOldestEvicted(t *testing.T) {
	r := NewRing(3)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		r.Append(mkEvent(id))
	}
	got := r.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if want := []string{"c", "d", "e"}; !equalIDs(got, want) {
		t.Errorf("snapshot = %v, want %v", ids(got), want)
	}
}

func TestRing_SnapshotIsolation(t *testing.T) {
	r := NewRing(4)
	r.Append(mkEvent("a"))
	snap := r.Snapshot()
	snap[0].ID = "mutated"
	if r.Snapshot()[0].ID != "a" {
		t.Errorf("snapshot mutation leaked into ring")
	}
}

func TestRing_ClearResetsLen(t *testing.T) {
	r := NewRing(4)
	r.Append(mkEvent("a"))
	r.Clear()
	if r.Len() != 0 {
		t.Errorf("len after Clear = %d, want 0", r.Len())
	}
	if len(r.Snapshot()) != 0 {
		t.Errorf("snapshot after Clear non-empty")
	}
	r.Append(mkEvent("z"))
	if got := r.Snapshot(); len(got) != 1 || got[0].ID != "z" {
		t.Errorf("post-clear append = %v", ids(got))
	}
}

func TestRing_FilterBySite(t *testing.T) {
	r := NewRing(8)
	r.Append(Event{V: 1, ID: "a", Kind: KindDump, Ctx: Context{Type: "fpm", Site: "one"}})
	r.Append(Event{V: 1, ID: "b", Kind: KindDump, Ctx: Context{Type: "fpm", Site: "two"}})
	r.Append(Event{V: 1, ID: "c", Kind: KindDump, Ctx: Context{Type: "cli", Site: "one"}})
	got := r.Filter(FilterOpts{Site: "one"})
	if !equalIDs(got, []string{"a", "c"}) {
		t.Errorf("filter site one = %v", ids(got))
	}
}

func TestRing_FilterByBranch(t *testing.T) {
	r := NewRing(8)
	r.Append(Event{V: 1, ID: "a", Kind: KindDump, Ctx: Context{Type: "fpm", Site: "acme"}})
	r.Append(Event{V: 1, ID: "b", Kind: KindDump, Ctx: Context{Type: "fpm", Site: "acme", Branch: "feature-x"}})
	r.Append(Event{V: 1, ID: "c", Kind: KindDump, Ctx: Context{Type: "fpm", Site: "acme", Branch: "feature-x"}})
	got := r.Filter(FilterOpts{Branch: "feature-x"})
	if !equalIDs(got, []string{"b", "c"}) {
		t.Errorf("filter branch feature-x = %v", ids(got))
	}
}

func TestRing_FilterByCtx(t *testing.T) {
	r := NewRing(8)
	r.Append(Event{V: 1, ID: "a", Kind: KindDump, Ctx: Context{Type: "fpm"}})
	r.Append(Event{V: 1, ID: "b", Kind: KindDump, Ctx: Context{Type: "cli"}})
	got := r.Filter(FilterOpts{Ctx: "cli"})
	if !equalIDs(got, []string{"b"}) {
		t.Errorf("filter ctx cli = %v", ids(got))
	}
}

func TestRing_FilterByKind(t *testing.T) {
	r := NewRing(8)
	r.Append(Event{V: 1, ID: "a", Kind: KindDump, Ctx: Context{Type: "fpm"}})
	r.Append(Event{V: 1, ID: "b", Kind: KindQuery, Ctx: Context{Type: "fpm"}})
	r.Append(Event{V: 1, ID: "c", Kind: KindQuery, Ctx: Context{Type: "cli"}})
	got := r.Filter(FilterOpts{Kind: KindQuery})
	if !equalIDs(got, []string{"b", "c"}) {
		t.Errorf("filter kind query = %v", ids(got))
	}
}

func TestRing_FilterSinceID(t *testing.T) {
	r := NewRing(8)
	for _, id := range []string{"a", "b", "c", "d"} {
		r.Append(mkEvent(id))
	}
	got := r.Filter(FilterOpts{SinceID: "b"})
	if !equalIDs(got, []string{"c", "d"}) {
		t.Errorf("filter since b = %v", ids(got))
	}
}

func TestRing_FilterLimitKeepsMostRecent(t *testing.T) {
	r := NewRing(8)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		r.Append(mkEvent(id))
	}
	got := r.Filter(FilterOpts{Limit: 2})
	if !equalIDs(got, []string{"d", "e"}) {
		t.Errorf("filter limit 2 = %v", ids(got))
	}
}

func TestRing_DefaultCapWhenZero(t *testing.T) {
	r := NewRing(0)
	if r.Cap() != DefaultCapacity {
		t.Errorf("cap = %d, want %d", r.Cap(), DefaultCapacity)
	}
}

func TestRing_ConcurrentAppendDoesntPanic(t *testing.T) {
	r := NewRing(64)
	done := make(chan struct{})
	for i := 0; i < 4; i++ {
		go func(seed int) {
			for j := 0; j < 200; j++ {
				r.Append(mkEvent(fmt.Sprintf("%d-%d", seed, j)))
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	if r.Len() != 64 {
		t.Errorf("len after concurrent append = %d, want 64 (saturated)", r.Len())
	}
}

func ids(es []Event) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.ID
	}
	return out
}

func equalIDs(es []Event, want []string) bool {
	if len(es) != len(want) {
		return false
	}
	for i, e := range es {
		if e.ID != want[i] {
			return false
		}
	}
	return true
}

func TestRing_RemoveKeepsOrderAndFreesSpace(t *testing.T) {
	r := NewRing(3)
	for _, id := range []string{"a", "b", "c", "d"} {
		r.Append(Event{ID: id, Ctx: Context{Test: id == "c"}})
	}
	r.Remove(func(e Event) bool { return e.Ctx.Test })
	got := r.Snapshot()
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "d" {
		t.Fatalf("after remove = %v, want b d", got)
	}
	r.Append(Event{ID: "e"})
	if got := r.Snapshot(); len(got) != 3 || got[2].ID != "e" {
		t.Errorf("after append = %v, want b d e", got)
	}
}

// One request's events are the ones it ran, plus a browser failure that names
// it as the request a fetch reached.
func TestRing_FilterByRID(t *testing.T) {
	r := NewRing(8)
	r.Append(Event{V: 1, ID: "a", Kind: KindQuery, Ctx: Context{Type: "fpm", RID: "r1"}})
	r.Append(Event{V: 1, ID: "b", Kind: KindQuery, Ctx: Context{Type: "fpm", RID: "r2"}})
	r.Append(Event{V: 1, ID: "c", Kind: KindBrowser, Ctx: Context{Type: "browser", RID: "page"}, Data: []byte(`{"type":"network","rid":"r1"}`)})
	r.Append(Event{V: 1, ID: "d", Kind: KindBrowser, Ctx: Context{Type: "browser", RID: "r1"}})
	if got := r.Filter(FilterOpts{RID: "r1"}); !equalIDs(got, []string{"a", "c", "d"}) {
		t.Errorf("filter rid r1 = %v", ids(got))
	}
}

// A ring resized while lerd-ui runs keeps the newest events that fit.
func TestRing_ResizeKeepsTheNewest(t *testing.T) {
	r := NewRing(4)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		r.Append(mkEvent(id))
	}
	r.Resize(2)
	if got := r.Snapshot(); !equalIDs(got, []string{"d", "e"}) || r.Cap() != 2 {
		t.Fatalf("shrunk = %v cap %d", ids(got), r.Cap())
	}
	r.Resize(3)
	r.Append(mkEvent("f"))
	r.Append(mkEvent("g"))
	if got := r.Snapshot(); !equalIDs(got, []string{"e", "f", "g"}) {
		t.Fatalf("grown = %v", ids(got))
	}
}

func TestRing_KeepsEventsAcrossARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug-events.db")
	r, err := OpenRing(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c"} {
		r.Append(mkEvent(id))
	}
	r.Close()
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("buffer file mode = %v, %v; captured SQL and payloads stay private", fi.Mode().Perm(), err)
	}

	next, err := OpenRing(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if got := ids(next.Snapshot()); fmt.Sprint(got) != "[b c]" {
		t.Fatalf("reopened = %v, want the newest that fit", got)
	}
}

func TestRing_ImportTakesInAnOldBufferFileAndDeletesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dumps-buffer.json")
	b, _ := json.Marshal([]Event{mkEvent("a"), mkEvent("b"), mkEvent("c")})
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := OpenRing(filepath.Join(filepath.Dir(path), "debug-events.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Import(path); err != nil {
		t.Fatal(err)
	}
	if got := ids(r.Snapshot()); fmt.Sprint(got) != "[b c]" {
		t.Fatalf("imported = %v, want the newest that fit", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("old buffer file still there: %v", err)
	}
}

func TestRing_ImportWithoutAnOldFileStartsEmpty(t *testing.T) {
	r := NewRing(4)
	if err := r.Import(filepath.Join(t.TempDir(), "missing.json")); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 0 {
		t.Fatalf("len = %d", r.Len())
	}
}

func TestRing_RequestIDsNamesEveryRequestWithEvents(t *testing.T) {
	r := NewRing(4)
	q := mkEvent("a")
	q.Ctx.RID = "r1"
	b := Event{V: 1, ID: "b", Kind: KindBrowser, Data: []byte(`{"type":"fetch","rid":"r2"}`)}
	r.Append(q)
	r.Append(b)
	r.Append(mkEvent("c"))
	got := r.RequestIDs()
	if len(got) != 2 || !got["r1"] || !got["r2"] {
		t.Fatalf("request ids = %v", got)
	}
}

func TestServer_ForgetRequestsDropsTheirEventsOnly(t *testing.T) {
	s, err := Listen(nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for _, e := range []Event{
		{V: ProtocolVersion, ID: "a", Kind: KindQuery, Ctx: Context{RID: "r1"}},
		{V: ProtocolVersion, ID: "b", Kind: KindBrowser, Data: []byte(`{"rid":"r1"}`)},
		{V: ProtocolVersion, ID: "c", Kind: KindQuery, Ctx: Context{RID: "r2"}},
	} {
		s.Push(e)
	}
	s.ForgetRequests([]string{"r1"})
	if got := ids(s.Snapshot()); fmt.Sprint(got) != "[c]" {
		t.Fatalf("left = %v", got)
	}
}

// Remove filters and rewrites under one lock, so an event appended while it runs
// is never overwritten by an older copy of the buffer.
func TestRing_RemoveKeepsEventsAppendedMeanwhile(t *testing.T) {
	r := NewRing(10000)
	for i := 0; i < 2000; i++ {
		r.Append(mkEvent(fmt.Sprintf("old%d", i)))
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 500; i++ {
			r.Append(mkEvent(fmt.Sprintf("new%d", i)))
		}
		close(done)
	}()
	for i := 0; i < 50; i++ {
		r.Remove(func(e Event) bool { return e.ID == "never" })
	}
	<-done
	if got := r.Len(); got != 2500 {
		t.Fatalf("len = %d, want 2500: events appended during Remove were lost", got)
	}
}

func ridEvent(id, rid string) Event {
	e := mkEvent(id)
	e.Ctx.RID = rid
	return e
}

// A kept request's events outlive the buffer, so a slowest-route bar days old
// still opens on something.
func TestRing_KeptRequestSurvivesEviction(t *testing.T) {
	r := NewRing(2)
	r.SetKeep(map[string]bool{"r1": true})
	r.Append(ridEvent("a", "r1"))
	r.Append(ridEvent("b", "r2"))
	r.Append(mkEvent("c"))
	r.Append(mkEvent("d"))
	if got := ids(r.Snapshot()); fmt.Sprint(got) != "[c d]" {
		t.Fatalf("snapshot = %v, pinned events stay out of the buffer", got)
	}
	if got := ids(r.Filter(FilterOpts{RID: "r1"})); fmt.Sprint(got) != "[a]" {
		t.Fatalf("filter r1 = %v", got)
	}
	if got := r.RequestIDs(); !got["r1"] || got["r2"] {
		t.Fatalf("request ids = %v", got)
	}
}

func TestRing_KeptBrowserEventPinsUnderTheRequestItReached(t *testing.T) {
	r := NewRing(1)
	r.SetKeep(map[string]bool{"r1": true})
	r.Append(Event{V: 1, ID: "a", Kind: KindBrowser, Ctx: Context{RID: "page"}, Data: []byte(`{"type":"fetch","rid":"r1"}`)})
	r.Append(mkEvent("b"))
	if got := ids(r.Filter(FilterOpts{RID: "r1"})); fmt.Sprint(got) != "[a]" {
		t.Fatalf("filter r1 = %v", got)
	}
}

func TestRing_SetKeepDropsRequestsNoLongerKept(t *testing.T) {
	r := NewRing(1)
	r.SetKeep(map[string]bool{"r1": true})
	r.Append(ridEvent("a", "r1"))
	r.Append(mkEvent("b"))
	r.SetKeep(map[string]bool{"r2": true})
	if got := r.Filter(FilterOpts{RID: "r1"}); len(got) != 0 {
		t.Fatalf("filter r1 = %v, want it gone once no longer kept", ids(got))
	}
	if r.RequestIDs()["r1"] {
		t.Fatal("r1 still listed as openable")
	}
}

func TestRing_SetKeepNeverRewritesEvents(t *testing.T) {
	r := NewRing(1)
	r.Append(ridEvent("a", "r1"))
	r.Append(ridEvent("b", "r2"))
	if _, err := r.db.Exec(`CREATE TEMP TABLE rewrites(n);
		CREATE TEMP TRIGGER count_rewrites AFTER UPDATE ON events BEGIN INSERT INTO rewrites VALUES(1); END`); err != nil {
		t.Fatal(err)
	}
	r.SetKeep(map[string]bool{"r1": true})
	r.SetKeep(map[string]bool{"r2": true})
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM rewrites`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("marking kept requests rewrote %d events", n)
	}
}

func TestRing_SetKeepHoldsARowStillReachedByAKeptRequest(t *testing.T) {
	r := NewRing(1)
	r.SetKeep(map[string]bool{"page": true, "r1": true})
	r.Append(Event{V: 1, ID: "a", Kind: KindBrowser, Ctx: Context{RID: "page"}, Data: []byte(`{"type":"fetch","rid":"r1"}`)})
	r.Append(mkEvent("b"))
	r.SetKeep(map[string]bool{"page": true})
	if got := ids(r.Filter(FilterOpts{RID: "page"})); fmt.Sprint(got) != "[a]" {
		t.Fatalf("filter page = %v", got)
	}
}

func TestRing_RemoveAndClearReachPinnedEvents(t *testing.T) {
	r := NewRing(1)
	r.SetKeep(map[string]bool{"r1": true, "r2": true})
	r.Append(ridEvent("a", "r1"))
	r.Append(ridEvent("b", "r2"))
	r.Append(mkEvent("c"))
	r.Remove(func(e Event) bool { return e.OfRequest("r1") })
	if got := r.RequestIDs(); got["r1"] || !got["r2"] {
		t.Fatalf("after remove = %v", got)
	}
	r.Clear()
	if got := r.RequestIDs(); len(got) != 0 {
		t.Fatalf("after clear = %v", got)
	}
}

func TestRing_ShrinkingPinsKeptEvents(t *testing.T) {
	r := NewRing(3)
	r.SetKeep(map[string]bool{"r1": true})
	r.Append(ridEvent("a", "r1"))
	r.Append(mkEvent("b"))
	r.Append(mkEvent("c"))
	r.Resize(1)
	if got := ids(r.Filter(FilterOpts{RID: "r1"})); fmt.Sprint(got) != "[a]" {
		t.Fatalf("filter r1 = %v", got)
	}
}

func TestRing_KeptRequestSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug-events.db")
	keep := map[string]bool{"r1": true}
	r, err := OpenRing(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	r.SetKeep(keep)
	r.Append(ridEvent("a", "r1"))
	r.Append(mkEvent("b"))
	r.Append(mkEvent("c"))
	r.Close()

	next, err := OpenRing(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	next.SetKeep(keep)
	if got := ids(next.Snapshot()); fmt.Sprint(got) != "[b c]" {
		t.Fatalf("buffer = %v", got)
	}
	if got := ids(next.Filter(FilterOpts{RID: "r1"})); fmt.Sprint(got) != "[a]" {
		t.Fatalf("filter r1 = %v", got)
	}
}

// FilterJSON hands back what Filter selects as the JSON array the API serves,
// copied from the stored rows rather than decoded and encoded again.
func TestRing_FilterJSONMatchesFilter(t *testing.T) {
	r := NewRing(3)
	for _, id := range []string{"a", "b", "c", "d"} {
		r.Append(ridEvent(id, "r"+id))
	}
	for _, opts := range []FilterOpts{{}, {Limit: 2}, {RID: "rc"}, {SinceID: "c"}, {Kind: "nope"}} {
		want, _ := json.Marshal(r.Filter(opts))
		if got := r.FilterJSON(opts); string(got) != string(want) {
			t.Errorf("FilterJSON(%+v) = %s, want %s", opts, got, want)
		}
	}
}

func kindEvent(id, site, kind string) Event {
	e := mkEvent(id)
	e.Ctx.Site, e.Kind = site, kind
	return e
}

// The Debug lens bar's badges come from here rather than from whatever a tab
// happened to load: a page view names its page but is not a row of its own,
// and a test run counts only when asked for.
func TestRing_CountsPerKindForASite(t *testing.T) {
	r := NewRing(10)
	r.Append(kindEvent("a", "acme", KindQuery))
	r.Append(kindEvent("b", "acme", KindQuery))
	r.Append(kindEvent("c", "acme", KindView))
	r.Append(kindEvent("d", "other", KindQuery))
	nav := kindEvent("e", "acme", KindBrowser)
	nav.Data = []byte(`{"type":"navigation"}`)
	r.Append(nav)
	test := kindEvent("f", "acme", KindQuery)
	test.Ctx.Test = true
	r.Append(test)

	got := r.Counts(FilterOpts{Site: "acme", HideTests: true})
	if fmt.Sprint(got) != "map[query:2 view:1]" {
		t.Errorf("counts = %v, want two queries and a view", got)
	}
	if got := r.Counts(FilterOpts{Site: "acme"}); got[KindQuery] != 3 {
		t.Errorf("with tests, queries = %d, want 3", got[KindQuery])
	}
	if got := r.TestCount(FilterOpts{Site: "acme"}); got != 1 {
		t.Errorf("hidden tests = %d, want 1", got)
	}
}

// A lens loads the newest page of its kind and pages back by id from there.
func TestRing_FilterPagesBackBeforeAnID(t *testing.T) {
	r := NewRing(10)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		r.Append(kindEvent(id, "acme", KindQuery))
	}
	r.Append(kindEvent("f", "acme", KindView))
	page := r.Filter(FilterOpts{Kind: KindQuery, Limit: 2})
	if fmt.Sprint(ids(page)) != "[d e]" {
		t.Fatalf("first page = %v", ids(page))
	}
	older := r.Filter(FilterOpts{Kind: KindQuery, Limit: 2, Before: page[0].ID})
	if fmt.Sprint(ids(older)) != "[b c]" {
		t.Errorf("older page = %v, want [b c]", ids(older))
	}
}

func TestRing_FilterHidesTestRuns(t *testing.T) {
	r := NewRing(10)
	r.Append(mkEvent("a"))
	test := mkEvent("b")
	test.Ctx.Test = true
	r.Append(test)
	if got := ids(r.Filter(FilterOpts{HideTests: true})); fmt.Sprint(got) != "[a]" {
		t.Errorf("filter = %v, want the test event left out", got)
	}
}

func TestRing_SitesListsEverySiteInTheBuffer(t *testing.T) {
	r := NewRing(10)
	r.Append(kindEvent("a", "beta", KindQuery))
	r.Append(kindEvent("b", "acme", KindView))
	r.Append(kindEvent("c", "beta", KindView))
	if got := r.Sites(); fmt.Sprint(got) != "[acme beta]" {
		t.Errorf("sites = %v", got)
	}
}

// An old buffer file is the only copy of its events until they are on disk,
// so it stays when they could not all be written or the ring is in memory.
func TestRing_ImportKeepsTheOldFileUnlessEveryEventLanded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dumps-buffer.json")
	b, _ := json.Marshal([]Event{mkEvent("a"), mkEvent("b")})
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	mem := NewRing(4)
	if err := mem.Import(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("an in-memory ring deleted the only copy of the old buffer")
	}

	disk, err := OpenRing(filepath.Join(dir, "debug-events.db"), 4)
	if err != nil {
		t.Fatal(err)
	}
	disk.Close() // every write now fails
	if err := disk.Import(path); err == nil {
		t.Error("import into a closed database reported success")
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("a failed import deleted the old buffer")
	}
}

// An import that fails part way writes nothing, so the retry on the next
// start cannot store the same events twice.
func TestRing_ImportIsAllOrNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dumps-buffer.json")
	bad := mkEvent("bad")
	bad.Data = []byte(`{"broken"`) // fails to encode, after "a" was written
	b, _ := json.Marshal([]Event{mkEvent("a")})
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := OpenRing(filepath.Join(dir, "debug-events.db"), 10)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.importEvents([]Event{mkEvent("a"), bad}); err == nil {
		t.Fatal("an event that cannot be encoded was imported")
	}
	if n := r.Len(); n != 0 {
		t.Errorf("after a failed import the buffer holds %d events, want none", n)
	}
	if err := r.Import(path); err != nil {
		t.Fatal(err)
	}
	if got := ids(r.Snapshot()); fmt.Sprint(got) != "[a]" {
		t.Errorf("after the retry = %v, want each event once", got)
	}
}

// The slowest-request refresh marks a request kept only after it ran, so a
// burst that fills the buffer first must not trim it away in the meantime.
func TestRing_TrimSparesEventsTheKeepRefreshHasNotSeen(t *testing.T) {
	r := NewRing(2)
	r.Append(ridEvent("slow", "r1"))
	for i := 0; i < 2*trimEvery; i++ {
		r.Append(mkEvent(fmt.Sprint(i)))
	}
	r.SetKeep(map[string]bool{"r1": true})
	if got := ids(r.Filter(FilterOpts{RID: "r1"})); fmt.Sprint(got) != "[slow]" {
		t.Fatalf("filter r1 = %v, want the request kept by a refresh after the burst", got)
	}
}

func TestRing_TrimDropsEventsOnceTheGraceHasPassed(t *testing.T) {
	clock := time.Now()
	ringNow = func() time.Time { return clock }
	t.Cleanup(func() { ringNow = time.Now })

	r := NewRing(2)
	r.Append(ridEvent("old", "r1"))
	for i := 0; i < trimEvery; i++ {
		r.Append(mkEvent(fmt.Sprint(i)))
	}
	clock = clock.Add(trimGrace)
	for i := 0; i < trimEvery; i++ {
		r.Append(mkEvent(fmt.Sprint("late", i)))
	}
	r.SetKeep(map[string]bool{"r1": true})
	if got := r.Filter(FilterOpts{RID: "r1"}); len(got) != 0 {
		t.Fatalf("filter r1 = %v, want it trimmed once older than the grace", ids(got))
	}
}

// A request still running past the grace keeps its early events while it goes
// on sending, since the keep refresh only learns of it once it has finished.
func TestRing_TrimSparesARequestStillSendingEvents(t *testing.T) {
	clock := time.Now()
	ringNow = func() time.Time { return clock }
	t.Cleanup(func() { ringNow = time.Now })

	r := NewRing(2)
	r.Append(ridEvent("early", "r1"))
	for i := 0; i < trimEvery; i++ {
		r.Append(mkEvent(fmt.Sprint(i)))
	}
	clock = clock.Add(trimGrace)
	r.Append(ridEvent("late", "r1"))
	for i := 0; i < trimEvery; i++ {
		r.Append(mkEvent(fmt.Sprint("after", i)))
	}
	r.SetKeep(map[string]bool{"r1": true})
	if got := ids(r.Filter(FilterOpts{RID: "r1"})); fmt.Sprint(got) != "[early late]" {
		t.Fatalf("filter r1 = %v, want both events of the long request", got)
	}
}

// Emptying the buffer lets SQLite hand out low seqs again, which the ages
// marked before must not cover.
func TestRing_TrimGraceStartsOverAfterARemove(t *testing.T) {
	clock := time.Now()
	ringNow = func() time.Time { return clock }
	t.Cleanup(func() { ringNow = time.Now })

	r := NewRing(2)
	for i := 0; i < 4*trimEvery; i++ {
		r.Append(mkEvent(fmt.Sprint(i)))
		if i == trimEvery {
			clock = clock.Add(trimGrace)
		}
	}
	r.Remove(func(Event) bool { return true })
	r.Append(ridEvent("slow", "r1"))
	for i := 0; i < 2*trimEvery; i++ {
		r.Append(mkEvent(fmt.Sprint("new", i)))
	}
	r.SetKeep(map[string]bool{"r1": true})
	if got := ids(r.Filter(FilterOpts{RID: "r1"})); fmt.Sprint(got) != "[slow]" {
		t.Fatalf("filter r1 = %v, want the request kept by a refresh after the burst", got)
	}
}

// A CLI worker keeps one rid for its whole life, and no slowest route can name
// it, so its steady stream must not hold its old events forever.
func TestRing_TrimDropsAWorkersOldEventsWhileItRuns(t *testing.T) {
	clock := time.Now()
	ringNow = func() time.Time { return clock }
	t.Cleanup(func() { ringNow = time.Now })

	worker := func(id string) Event {
		e := ridEvent(id, "w1")
		e.Ctx.Type = "cli"
		return e
	}
	r := NewRing(2)
	r.Append(worker("early"))
	for i := 0; i < trimEvery; i++ {
		r.Append(mkEvent(fmt.Sprint(i)))
	}
	clock = clock.Add(trimGrace)
	r.Append(worker("late"))
	for i := 0; i < trimEvery; i++ {
		r.Append(mkEvent(fmt.Sprint("after", i)))
	}
	r.SetKeep(map[string]bool{"w1": true})
	if got := ids(r.Filter(FilterOpts{RID: "w1"})); fmt.Sprint(got) != "[late]" {
		t.Fatalf("filter w1 = %v, want only the worker's event inside the grace", got)
	}
}

// Kept requests hold their events past the buffer, but together no more than
// the buffer itself; the biggest go first, so one huge page costs one request.
func TestRing_KeptRequestsHoldNoMoreThanTheBufferBiggestFirst(t *testing.T) {
	r := NewRing(3)
	r.SetKeep(map[string]bool{"r1": true, "r2": true, "r3": true})
	r.Append(ridEvent("a1", "r1"))
	r.Append(ridEvent("b1", "r2"))
	r.Append(ridEvent("b2", "r2"))
	r.Append(ridEvent("b3", "r2"))
	r.Append(ridEvent("c1", "r3"))
	r.SetKeep(map[string]bool{"r1": true, "r2": true, "r3": true})
	got := r.RequestIDs()
	if !got["r1"] || got["r2"] || !got["r3"] {
		t.Fatalf("request ids = %v, want the biggest kept request dropped whole", got)
	}
}

// passGrace runs two trims trimGrace apart, so the rows appended before the
// call are old enough for a trim to drop.
func passGrace(t *testing.T, r *Ring, keep map[string]bool) {
	clock := time.Now()
	ringNow = func() time.Time { return clock }
	t.Cleanup(func() { ringNow = time.Now })
	r.SetKeep(keep)
	clock = clock.Add(trimGrace)
	r.SetKeep(keep)
}

func cliEvent(id, rid string) Event {
	e := ridEvent(id, rid)
	e.Ctx.Type = "cli"
	return e
}

// One CLI run rendering thousands of views must not push every web request
// out: past a quarter of the buffer its own oldest events go.
func TestRing_OneRequestHoldsAtMostAQuarterOfTheBuffer(t *testing.T) {
	r := NewRing(8)
	for i := 0; i < 5; i++ {
		r.Append(cliEvent(fmt.Sprint("c", i), "w1"))
	}
	r.Append(ridEvent("a", "r1"))
	passGrace(t, r, nil)
	if got := ids(r.Snapshot()); fmt.Sprint(got) != "[c3 c4 a]" {
		t.Fatalf("snapshot = %v, want the CLI run cut to its newest two", got)
	}
}

// When the buffer overflows, finished CLI runs give way before web requests.
func TestRing_CLIEventsGoBeforeWebOnes(t *testing.T) {
	r := NewRing(4)
	r.Append(ridEvent("a", "ra"))
	r.Append(ridEvent("b", "rb"))
	r.Append(cliEvent("c1", "w1"))
	r.Append(cliEvent("c2", "w2"))
	r.Append(cliEvent("c3", "w3"))
	r.Append(ridEvent("d", "rd"))
	passGrace(t, r, nil)
	if got := ids(r.Snapshot()); fmt.Sprint(got) != "[a b c3 d]" {
		t.Fatalf("snapshot = %v, want the oldest CLI events dropped first", got)
	}
}

func TestRing_DroppingKeptRequestsShrinksTheFile(t *testing.T) {
	r, err := OpenRing(filepath.Join(t.TempDir(), "debug-events.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	big := []byte(`{"blob":"` + strings.Repeat("x", 64<<10) + `"}`)
	keep := map[string]bool{}
	for i := 0; i < 50; i++ {
		rid := fmt.Sprint("r", i)
		keep[rid] = true
		e := ridEvent(rid, rid)
		e.Data = big
		r.Append(e)
	}
	r.SetKeep(keep)
	var pages int
	if err := r.db.QueryRow(`PRAGMA page_count`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if pages > 200 {
		t.Fatalf("page_count = %d after dropping 48 of 50 large requests, want the file shrunk", pages)
	}
}

// The kept set lives in its own table, so an index on the old column is only
// another write per event; a buffer from an older lerd loses it on open.
func TestRing_OpenDropsTheUnusedKeptIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug-events.db")
	r, err := OpenRing(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_kept ON events(kept)`); err != nil {
		t.Fatal(err)
	}
	r.Close()
	r, err = OpenRing(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'idx_events_kept'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("idx_events_kept still exists after open")
	}
}

func fetchEvent(id, page, reached string) Event {
	return Event{V: 1, ID: id, Kind: KindBrowser, Ctx: Context{RID: page}, Data: []byte(`{"type":"fetch","rid":"` + reached + `"}`)}
}

func rowExists(t *testing.T, r *Ring, id string) bool {
	t.Helper()
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM events WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// Dropping the newest rows lets SQLite hand their seqs out again, which the
// ages marked before must not cover.
func TestRing_TrimGraceStartsOverAfterDroppingKeptRequests(t *testing.T) {
	r := NewRing(8)
	r.SetKeep(map[string]bool{"r1": true})
	for i := 0; i < 9; i++ {
		r.Append(ridEvent(fmt.Sprint("k", i), "r1"))
	}
	passGrace(t, r, map[string]bool{"r1": true})
	for i := 0; i < 9; i++ {
		r.Append(mkEvent(fmt.Sprint("n", i)))
	}
	r.Resize(8)
	if !rowExists(t, r, "n0") {
		t.Fatal("n0 was trimmed before its grace had passed")
	}
}

func TestRing_TrimShrinksTheFileWithoutDroppingKeptRequests(t *testing.T) {
	r, err := OpenRing(filepath.Join(t.TempDir(), "debug-events.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	big := []byte(`{"blob":"` + strings.Repeat("x", 64<<10) + `"}`)
	for i := 0; i < 50; i++ {
		e := mkEvent(fmt.Sprint("e", i))
		e.Data = big
		r.Append(e)
	}
	passGrace(t, r, nil)
	var pages int
	if err := r.db.QueryRow(`PRAGMA page_count`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if pages > 200 {
		t.Fatalf("page_count = %d after trimming 48 of 50 large events, want the file shrunk", pages)
	}
}

// A browser row a kept page and a kept fetch both hold is one event, not two.
func TestRing_KeptCeilingCountsASharedRowOnce(t *testing.T) {
	r := NewRing(2)
	r.SetKeep(map[string]bool{"page": true, "fetch": true})
	r.Append(fetchEvent("a", "page", "fetch"))
	r.Append(ridEvent("f1", "fetch"))
	r.SetKeep(map[string]bool{"page": true, "fetch": true})
	if !rowExists(t, r, "a") || !rowExists(t, r, "f1") {
		t.Fatal("a kept request was dropped though the two distinct events fit")
	}
}

func TestRing_DroppingAKeptRequestSparesRowsAnotherStillHolds(t *testing.T) {
	r := NewRing(2)
	keep := map[string]bool{"page": true, "fetch": true}
	r.SetKeep(keep)
	r.Append(fetchEvent("a", "page", "fetch"))
	r.Append(ridEvent("p1", "page"))
	r.Append(ridEvent("p2", "page"))
	r.Append(ridEvent("f1", "fetch"))
	r.SetKeep(keep)
	if got := ids(r.Filter(FilterOpts{RID: "fetch"})); fmt.Sprint(got) != "[a f1]" {
		t.Fatalf("filter fetch = %v, want the browser row it reached kept", got)
	}
	if rowExists(t, r, "p1") {
		t.Fatal("the biggest kept request was not dropped")
	}
}

// Dropping a page whose browser rows two kept fetches still hold frees none of
// them, so the ceiling must be checked again before it stops dropping.
func TestRing_KeptCeilingRecountsAfterADropThatFreesNothing(t *testing.T) {
	r := NewRing(2)
	keep := map[string]bool{"page": true, "f1": true, "f2": true}
	r.SetKeep(keep)
	r.Append(fetchEvent("a", "page", "f1"))
	r.Append(fetchEvent("b", "page", "f2"))
	r.Append(ridEvent("e1", "f1"))
	r.Append(ridEvent("e2", "f2"))
	r.SetKeep(keep)
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n > 2 {
		t.Fatalf("%d kept events left, want at most the buffer's 2", n)
	}
}
