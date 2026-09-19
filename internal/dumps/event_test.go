package dumps

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEvent_Valid(t *testing.T) {
	cases := []struct {
		name string
		ev   Event
		want bool
	}{
		{"complete", Event{V: 1, ID: "abc", Kind: KindDump}, true},
		{"missing v", Event{ID: "abc", Kind: KindDump}, false},
		{"wrong v", Event{V: 999, ID: "abc", Kind: KindDump}, false},
		{"missing id", Event{V: 1, Kind: KindDump}, false},
		{"missing kind", Event{V: 1, ID: "abc"}, false},
	}
	for _, c := range cases {
		if got := c.ev.Valid(); got != c.want {
			t.Errorf("%s: Valid() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestEvent_RoundTripJSON(t *testing.T) {
	src := Event{
		V:    1,
		ID:   "01HZ",
		TS:   "2026-05-10T12:00:00.000Z",
		Kind: "dump",
		Ctx: Context{
			Type:    "fpm",
			Site:    "acme",
			Domain:  "acme.test",
			Request: "GET /",
			PID:     42,
		},
		Src:   Source{File: "/x.php", Line: 12},
		Label: "$user",
		Text:  "App\\Models\\User",
		Tree:  json.RawMessage(`{"kind":"object"}`),
	}
	b, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Event
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != src.ID || got.Ctx.Site != "acme" || got.Src.Line != 12 {
		t.Errorf("roundtrip drift: got %+v", got)
	}
}

func TestEvent_OmitsEmptyFields(t *testing.T) {
	ev := Event{V: 1, ID: "x", Kind: "dump"}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, k := range []string{`"label"`, `"text"`, `"tree"`, `"data"`, `"trunc"`, `"site"`, `"domain"`} {
		if strings.Contains(s, k) {
			t.Errorf("expected %s omitted, got %s", k, s)
		}
	}
}

func TestEvent_Query(t *testing.T) {
	q := Event{
		V:    1,
		ID:   "q1",
		Kind: KindQuery,
		Src:  Source{File: "/app/Models/User.php", Line: 30},
		Data: json.RawMessage(`{"sql":"select * from users where id = ?","bindings":[7],"time_ms":1.4,"connection":"mysql","rw_type":"read"}`),
	}
	got, ok := q.Query()
	if !ok {
		t.Fatal("Query() ok = false, want true")
	}
	if got.SQL != "select * from users where id = ?" || got.TimeMS != 1.4 || got.Connection != "mysql" || got.RWType != "read" {
		t.Errorf("decoded query drift: %+v", got)
	}
	if len(got.Bindings) != 1 {
		t.Errorf("bindings len = %d, want 1", len(got.Bindings))
	}

	if _, ok := (Event{Kind: KindDump, Data: q.Data}).Query(); ok {
		t.Error("Query() on a dump returned ok = true")
	}
	if _, ok := (Event{Kind: KindQuery, Data: json.RawMessage(`{`)}).Query(); ok {
		t.Error("Query() on malformed data returned ok = true")
	}
}

// TestNormalized_FillsWhatACallerCannotKnow covers the shape a poster outside
// the bridge sends: a message and a site, with the protocol fields defaulted.
func TestNormalized_FillsWhatACallerCannotKnow(t *testing.T) {
	when := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
	got, err := Event{Text: "deploy finished", Ctx: Context{Site: "acme"}}.Normalized(when, "id-1")
	if err != nil {
		t.Fatalf("Normalized: %v", err)
	}
	if got.V != ProtocolVersion || got.ID != "id-1" || got.Kind != KindDump {
		t.Errorf("event = %+v, want the protocol fields defaulted", got)
	}
	if got.TS != "2026-09-20T10:30:00.000Z" {
		t.Errorf("ts = %q, want the time it arrived", got.TS)
	}
	if got.Ctx.Type != "cli" || got.Ctx.Site != "acme" {
		t.Errorf("ctx = %+v, want a cli context and the site it named", got.Ctx)
	}
}

// TestNormalized_KeepsWhatTheCallerSet checks a caller that fills the envelope
// itself is left alone, so an agent replaying captured events keeps their ids.
func TestNormalized_KeepsWhatTheCallerSet(t *testing.T) {
	in := Event{V: 1, ID: "own", TS: "2026-01-01T00:00:00.000Z", Kind: KindLog, Ctx: Context{Type: "fpm"}, Data: json.RawMessage(`{"level":"error"}`)}
	got, err := in.Normalized(time.Now(), "generated")
	if err != nil {
		t.Fatalf("Normalized: %v", err)
	}
	if got.ID != "own" || got.TS != in.TS || got.Kind != KindLog || got.Ctx.Type != "fpm" {
		t.Errorf("event = %+v, want the caller's own envelope", got)
	}
}

// TestNormalized_RejectsAnEmptyEvent keeps a row that says nothing out of the
// buffer, since the poster gets an error back rather than a silent no-op.
func TestNormalized_RejectsAnEmptyEvent(t *testing.T) {
	if _, err := (Event{Ctx: Context{Site: "acme"}}).Normalized(time.Now(), "id-1"); err == nil {
		t.Error("an event with no text, label or data must be refused")
	}
}
