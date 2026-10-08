package dumps

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// DefaultCapacity is how many events the ring keeps when dumps.buffer is unset,
// matching config.DefaultDumpsBuffer. An event-heavy request emits thousands, so
// 3000 let one push the requests before it out within seconds.
const DefaultCapacity = 5000

// Ring is a fixed-size ring buffer of Events safe for concurrent use.
// Snapshots are taken under a read lock and returned in insertion order.
type Ring struct {
	mu   sync.RWMutex
	buf  []Event
	head int // next write index
	size int // populated entries, 0..cap
	cap  int
	// keep names the requests whose events move to pinned, oldest first,
	// instead of being dropped when the buffer evicts them: each route's
	// slowest, which the dashboard links to for days after the buffer turns over.
	keep   map[string]bool
	pinned []Event
}

// NewRing returns a ring with the given capacity. Non-positive capacity is
// replaced with DefaultCapacity.
func NewRing(capacity int) *Ring {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Ring{buf: make([]Event, capacity), cap: capacity}
}

// Resize changes how many events the ring keeps, carrying over the newest
// ones that fit, so the size can change without restarting lerd-ui.
func (r *Ring) Resize(capacity int) {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.snapshot()
	if len(kept) > capacity {
		for _, e := range kept[:len(kept)-capacity] {
			r.pin(e)
		}
		kept = kept[len(kept)-capacity:]
	}
	buf := make([]Event, capacity)
	copy(buf, kept)
	r.buf, r.cap, r.size, r.head = buf, capacity, len(kept), len(kept)%capacity
}

// Append stores e, evicting the oldest entry once the ring is full.
func (r *Ring) Append(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size == r.cap {
		r.pin(r.buf[r.head])
	}
	r.buf[r.head] = e
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
}

// SetKeep replaces the set of requests whose events outlive the buffer, and
// drops the pinned events of any request no longer in it.
func (r *Ring) SetKeep(keep map[string]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keep = keep
	r.pinned = r.removePinned(func(e Event) bool { return !r.kept(e) })
}

// pin sets an evicted event aside when it belongs to a kept request.
func (r *Ring) pin(e Event) {
	if r.kept(e) {
		r.pinned = append(r.pinned, e)
	}
}

// kept reports whether e ran in a kept request or is a browser event naming
// one as the request it reached.
func (r *Ring) kept(e Event) bool {
	return r.keep[e.Ctx.RID] || r.keep[e.reachedRID()]
}

// removePinned returns the pinned events drop does not match, in order.
func (r *Ring) removePinned(drop func(Event) bool) []Event {
	var left []Event
	for _, e := range r.pinned {
		if !drop(e) {
			left = append(left, e)
		}
	}
	return left
}

// Snapshot returns a copy of the ring contents in insertion order (oldest
// first). The returned slice is independent of the ring's backing array.
func (r *Ring) Snapshot() []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshot()
}

func (r *Ring) snapshot() []Event {
	out := make([]Event, 0, r.size)
	if r.size < r.cap {
		out = append(out, r.buf[:r.size]...)
		return out
	}
	out = append(out, r.buf[r.head:]...)
	out = append(out, r.buf[:r.head]...)
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cap
}

// Clear empties the ring. Subsequent Snapshot() returns an empty slice.
func (r *Ring) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.head = 0
	r.size = 0
	r.pinned = nil
	for i := range r.buf {
		r.buf[i] = Event{}
	}
}

// Remove drops every entry drop matches, keeping the rest in order and freeing
// their slots for new events.
func (r *Ring) Remove(drop func(Event) bool) {
	// One lock across the read and the rewrite, or an event appended between
	// them would be overwritten by the older copy.
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := make([]Event, 0, r.size)
	for _, e := range r.snapshot() {
		if !drop(e) {
			kept = append(kept, e)
		}
	}
	r.pinned = r.removePinned(drop)
	clear(r.buf)
	copy(r.buf, kept)
	r.size = len(kept)
	r.head = len(kept) % r.cap
}

// Save writes the ring to path, so a restarted lerd-ui can pick up where this
// one stopped. Owner-only: events carry SQL bindings and request payloads.
// Pinned events go first, as the oldest, so a Load with the same keep set
// evicts them straight back into pinned.
func (r *Ring) Save(path string) error {
	r.mu.RLock()
	events := append(append([]Event(nil), r.pinned...), r.snapshot()...)
	r.mu.RUnlock()
	b, err := json.Marshal(events)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load appends the events Save wrote to path, oldest first, so the newest that
// fit survive a smaller ring. A missing file is an empty buffer, not an error.
func (r *Ring) Load(path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var events []Event
	if err := json.Unmarshal(b, &events); err != nil {
		return err
	}
	for _, e := range events {
		r.Append(e)
	}
	return nil
}

// RequestIDs is the set of requests with at least one event in the ring: the
// ones a recent request's Inspect can open on something.
func (r *Ring) RequestIDs() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]bool{}
	for _, e := range append(append([]Event(nil), r.pinned...), r.snapshot()...) {
		if e.Ctx.RID != "" {
			out[e.Ctx.RID] = true
		}
		if rid := e.reachedRID(); rid != "" {
			out[rid] = true
		}
	}
	return out
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
	// RID keeps one request's events: those it ran, and a browser event that
	// names it as the request a fetch reached.
	RID string
	// SinceID drops events whose ID is lexicographically <= SinceID.
	SinceID string
	// Limit caps the returned slice to the most recent N entries.
	// Zero or negative means no limit.
	Limit int
}

// Filter returns a Snapshot filtered by opts, preserving insertion order.
func (r *Ring) Filter(opts FilterOpts) []Event {
	r.mu.RLock()
	snap := r.snapshot()
	if opts.RID != "" {
		snap = append(append([]Event(nil), r.pinned...), snap...)
	}
	r.mu.RUnlock()
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
		if opts.RID != "" && !e.OfRequest(opts.RID) {
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
	return out
}
