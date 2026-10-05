package dumps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

// DefaultCapacity is the maximum number of events the ring keeps before it
// overwrites the oldest entry. A single N+1 request can emit well over a
// thousand query events, so the old 500-line cap could not even retain one
// request's worth for analyze_queries to read; sized up so a fresh capture of
// one pathological request survives long enough to be analyzed. A page sends
// some 60 events, and with their traces kept as frame numbers (see frames.go)
// an event is under 1 KB, so this keeps around a thousand requests.
const DefaultCapacity = 60000

// Ring is a fixed-size ring buffer of Events safe for concurrent use.
// Snapshots are taken under a read lock and returned in insertion order.
type Ring struct {
	mu  sync.RWMutex
	buf []Event
	// traces holds each slot's trace as frame numbers, nil for none.
	traces [][]uint32
	frames *frameTable
	head   int // next write index
	size   int // populated entries, 0..cap
	cap    int
}

// NewRing returns a ring with the given capacity. Non-positive capacity is
// replaced with DefaultCapacity.
func NewRing(capacity int) *Ring {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Ring{buf: make([]Event, capacity), traces: make([][]uint32, capacity), frames: newFrameTable(), cap: capacity}
}

// Append stores e, evicting the oldest entry once the ring is full.
// It returns the event as a reader gets it, a trace the event only named by
// key put back, for whoever passes it on live.
func (r *Ring) Append(e Event) Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ids := r.frames.strip(e)
	r.buf[r.head], r.traces[r.head] = stored, ids
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
	if ids == nil || !keyedOnly(e) {
		return e
	}
	return r.frames.restore(stored, ids)
}

// keyedOnly reports whether e named its trace by key alone.
func keyedOnly(e Event) bool {
	return bytes.Contains(e.Data, []byte(`"trace_key"`)) && !bytes.Contains(e.Data, []byte(`"trace":`))
}

// Snapshot returns a copy of the ring contents in insertion order (oldest
// first), traces included. The returned slice is independent of the ring's
// backing array.
func (r *Ring) Snapshot() []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	slots := r.slots()
	out := make([]Event, len(slots))
	for i, s := range slots {
		out[i] = r.frames.restore(s.event, s.trace)
	}
	return out
}

// Lite is Snapshot without the traces, for a reader that lists or groups
// events and puts the traces back with Expand on the few it shows.
func (r *Ring) Lite() []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	slots := r.slots()
	out := make([]Event, len(slots))
	for i, s := range slots {
		out[i] = s.event
		out[i].lite = s.trace
	}
	return out
}

// Expand puts back the traces of events Lite returned.
func (r *Ring) Expand(evs []Event) []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Event, len(evs))
	for i, e := range evs {
		ids := e.lite
		e.lite = nil
		out[i] = r.frames.restore(e, ids)
	}
	return out
}

// Shared puts back the traces of events Lite returned as a table instead:
// each distinct trace once in traces, and an event's data naming its entry as
// trace_ref. A request that ran the same query a thousand times then carries
// its trace once, not a thousand times.
func (r *Ring) Shared(evs []Event) ([]Event, []json.RawMessage) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Event, len(evs))
	var traces []json.RawMessage
	index := map[string]int{}
	for i, e := range evs {
		ids := e.lite
		e.lite = nil
		out[i] = e
		if ids == nil {
			continue
		}
		key := fmt.Sprint(ids)
		ref, ok := index[key]
		if !ok {
			ref = len(traces)
			index[key] = ref
			traces = append(traces, r.frames.frames(ids))
		}
		var data map[string]json.RawMessage
		if json.Unmarshal(e.Data, &data) != nil {
			continue
		}
		data["trace_ref"] = json.RawMessage(strconv.Itoa(ref))
		if full, err := json.Marshal(data); err == nil {
			out[i].Data = full
		}
	}
	return out, traces
}

type slot struct {
	event Event
	trace []uint32
}

// slots copies the populated slots in insertion order; the caller holds the lock.
func (r *Ring) slots() []slot {
	out := make([]slot, 0, r.size)
	add := func(from, to int) {
		for i := from; i < to; i++ {
			out = append(out, slot{r.buf[i], r.traces[i]})
		}
	}
	if r.size < r.cap {
		add(0, r.size)
		return out
	}
	add(r.head, r.cap)
	add(0, r.head)
	return out
}

// Len returns the number of populated entries.
func (r *Ring) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

// Cap returns the maximum number of entries the ring can hold.
func (r *Ring) Cap() int {
	return r.cap
}

// Clear empties the ring. Subsequent Snapshot() returns an empty slice.
func (r *Ring) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.head = 0
	r.size = 0
	clear(r.buf)
	clear(r.traces)
	r.frames = newFrameTable()
}

// Remove drops every entry drop matches, keeping the rest in order and freeing
// their slots for new events.
func (r *Ring) Remove(drop func(Event) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := make([]slot, 0, r.size)
	for _, s := range r.slots() {
		if !drop(s.event) {
			kept = append(kept, s)
		}
	}
	clear(r.buf)
	clear(r.traces)
	for i, s := range kept {
		r.buf[i], r.traces[i] = s.event, s.trace
	}
	r.size = len(kept)
	r.head = len(kept) % r.cap
}

// FilterOpts narrows a Snapshot. Zero-value fields are ignored.
type FilterOpts struct {
	// Site exact-matches Ctx.Site when non-empty.
	Site string
	// Branch exact-matches Ctx.Branch when non-empty, isolating one git
	// worktree's events from the parent site they share a Site name with.
	Branch string
	// Ctx exact-matches Ctx.Type ("fpm" or "cli") when non-empty.
	Ctx string
	// Kind exact-matches Event.Kind when non-empty (e.g. "query", "dump").
	Kind string
	// SinceID drops events whose ID is lexicographically <= SinceID.
	SinceID string
	// Limit caps the returned slice to the most recent N entries.
	// Zero or negative means no limit.
	Limit int
}

// Filter returns a Snapshot filtered by opts, preserving insertion order.
func (r *Ring) Filter(opts FilterOpts) []Event {
	snap := r.Lite()
	out := make([]Event, 0, len(snap))
	for _, e := range snap {
		if opts.Site != "" && e.Ctx.Site != opts.Site {
			continue
		}
		if opts.Branch != "" && e.Ctx.Branch != opts.Branch {
			continue
		}
		if opts.Ctx != "" && e.Ctx.Type != opts.Ctx {
			continue
		}
		if opts.Kind != "" && e.Kind != opts.Kind {
			continue
		}
		if opts.SinceID != "" && e.ID <= opts.SinceID {
			continue
		}
		out = append(out, e)
	}
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[len(out)-opts.Limit:]
	}
	return r.Expand(out)
}
