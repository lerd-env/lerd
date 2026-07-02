package reqstats

// ring is a fixed-capacity rolling buffer of the most recent request-time
// samples for one route (or a site's overall traffic). It overwrites the oldest
// sample when full, so memory per route stays bounded regardless of volume.
type ring struct {
	buf  []float64
	next int
	full bool
}

func newRing(cap int) *ring { return &ring{buf: make([]float64, cap)} }

func (r *ring) add(v float64) {
	r.buf[r.next] = v
	r.next++
	if r.next == len(r.buf) {
		r.next = 0
		r.full = true
	}
}

func (r *ring) len() int {
	if r.full {
		return len(r.buf)
	}
	return r.next
}

func (r *ring) values() []float64 {
	return r.buf[:r.len()]
}
