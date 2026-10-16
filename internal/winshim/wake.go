package winshim

import (
	"sync"
	"time"
)

// Waker boots the distro again when a site is looked up while it is down, as
// after a `wsl --shutdown`, a `wsl --update` or a crash. It acts on demand
// rather than on a timer so a deliberate shutdown stays down until the user
// opens a site, and probes at most once per Interval because a page load
// fires a burst of lookups.
type Waker struct {
	Alive    func() bool // reports whether lerd's services answer
	Boot     func()      // starts the distro, and with it lerd's services
	Now      func() time.Time
	Interval time.Duration

	mu   sync.Mutex
	last time.Time
}

// Check probes the distro unless it did so within Interval, and boots it when
// it is down. Callers run it in a goroutine so the lookup is answered at once.
func (w *Waker) Check() {
	w.mu.Lock()
	now := w.Now()
	if !w.last.IsZero() && now.Sub(w.last) < w.Interval {
		w.mu.Unlock()
		return
	}
	w.last = now
	w.mu.Unlock()
	if !w.Alive() {
		w.Boot()
	}
}
