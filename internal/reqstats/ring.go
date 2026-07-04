package reqstats

// sample is one request-time measurement plus when it was recorded, so stale
// outliers can be filtered out at read time even while they sit in the buffer.
type sample struct {
	ms float64
	at int64 // unix nanoseconds
}

// ring is a fixed-capacity rolling buffer of the most recent request-time
// samples for one route (or a site's overall traffic). It overwrites the oldest
// sample when full, so memory per route stays bounded regardless of volume.
type ring struct {
	buf  []sample
	next int
	full bool
}

func newRing(cap int) *ring { return &ring{buf: make([]sample, cap)} }

func (r *ring) add(ms float64, at int64) {
	r.buf[r.next] = sample{ms: ms, at: at}
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

// values returns the ms of every retained sample recorded at or after cutoff (in
// unix nanoseconds). A cutoff <= 0 returns all retained samples. Order is not
// preserved, which is fine for the median and percentile callers that sort.
func (r *ring) values(cutoff int64) []float64 {
	n := r.len()
	out := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		if s := r.buf[i]; cutoff <= 0 || s.at >= cutoff {
			out = append(out, s.ms)
		}
	}
	return out
}
