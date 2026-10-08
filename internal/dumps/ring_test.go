package dumps

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
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

func TestRing_SaveLoadKeepsEventsAcrossARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dumps-buffer.json")
	r := NewRing(4)
	for _, id := range []string{"a", "b", "c"} {
		r.Append(mkEvent(id))
	}
	if err := r.Save(path); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("saved file mode = %v, %v; captured SQL and payloads stay private", fi.Mode().Perm(), err)
	}

	next := NewRing(2)
	if err := next.Load(path); err != nil {
		t.Fatal(err)
	}
	if got := ids(next.Snapshot()); fmt.Sprint(got) != "[b c]" {
		t.Fatalf("loaded = %v, want the newest that fit", got)
	}
}

func TestRing_LoadWithoutASavedFileStartsEmpty(t *testing.T) {
	r := NewRing(4)
	if err := r.Load(filepath.Join(t.TempDir(), "missing.json")); err != nil {
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
