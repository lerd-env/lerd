package dumps

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func traced(id string, frames ...string) Event {
	trace := make([]map[string]any, 0, len(frames))
	for _, f := range frames {
		trace = append(trace, map[string]any{"file": f, "line": 1, "func": "run"})
	}
	data, _ := json.Marshal(map[string]any{"sql": "select " + id, "trace": trace})
	return Event{V: 1, ID: id, Kind: KindQuery, Data: data}
}

func dataOf(t *testing.T, e Event) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(e.Data, &m); err != nil {
		t.Fatalf("data %s: %v", e.Data, err)
	}
	return m
}

// Traces come back whole while the ring keeps each distinct frame once.
func TestRing_KeepsEachFrameOnceAndGivesTracesBackWhole(t *testing.T) {
	r := NewRing(10)
	in := []Event{traced("a", "/app/routes/web.php", "/app/vendor/Kernel.php"), traced("b", "/app/Cart.php", "/app/vendor/Kernel.php"), traced("c")}
	for _, e := range in {
		r.Append(e)
	}
	if n := len(r.frames.list); n != 3 {
		t.Fatalf("%d frames kept, want 3 distinct", n)
	}
	got := r.Snapshot()
	for i := range in {
		if want := dataOf(t, in[i]); !reflect.DeepEqual(dataOf(t, got[i]), want) {
			t.Errorf("event %s came back as %s, want %s", in[i].ID, got[i].Data, in[i].Data)
		}
	}
}

func TestRing_LiteLeavesTracesOutUntilExpanded(t *testing.T) {
	r := NewRing(10)
	r.Append(traced("a", "/app/Cart.php"))
	lite := r.Lite()
	if _, ok := dataOf(t, lite[0])["trace"]; ok {
		t.Fatalf("lite event carries its trace: %s", lite[0].Data)
	}
	if trace := dataOf(t, r.Expand(lite)[0])["trace"].([]any); len(trace) != 1 {
		t.Fatalf("expanded trace = %v", trace)
	}
}

func TestRing_RemoveAndFilterKeepTheTraces(t *testing.T) {
	r := NewRing(10)
	r.Append(traced("a", "/app/A.php"))
	r.Append(Event{V: 1, ID: "d", Kind: "dump"})
	r.Append(traced("b", "/app/B.php"))
	r.Remove(func(e Event) bool { return e.Kind == "dump" })
	got := r.Filter(FilterOpts{Kind: KindQuery})
	if len(got) != 2 || len(dataOf(t, got[1])["trace"].([]any)) != 1 {
		t.Fatalf("after remove: %+v", got)
	}
	r.Clear()
	if len(r.frames.list) != 0 || r.Len() != 0 {
		t.Fatalf("clear left %d frames, %d events", len(r.frames.list), r.Len())
	}
}

// Past the cap a trace is kept as it came rather than growing the table.
func TestRing_FullFrameTableKeepsTracesAsTheyCame(t *testing.T) {
	r := NewRing(10)
	r.frames.list = make([]json.RawMessage, maxFrames)
	e := traced("a", fmt.Sprintf("/app/%d.php", 1))
	r.Append(e)
	if string(r.Snapshot()[0].Data) != string(e.Data) {
		t.Fatalf("full table rewrote the event: %s", r.Snapshot()[0].Data)
	}
}
