package winshim

import (
	"testing"
	"time"
)

type fakeDistro struct {
	up     bool
	boots  int
	checks int
}

func (f *fakeDistro) waker(now *time.Time) *Waker {
	return &Waker{
		Alive:    func() bool { f.checks++; return f.up },
		Boot:     func() { f.boots++; f.up = true },
		Now:      func() time.Time { return *now },
		Interval: 30 * time.Second,
	}
}

// A distro stopped by `wsl --shutdown` comes back the first time a site is
// looked up, not before, so a deliberate shutdown is not undone on its own.
func TestWaker_bootsAStoppedDistroOnDemand(t *testing.T) {
	now := time.Unix(1000, 0)
	d := &fakeDistro{}
	w := d.waker(&now)
	w.Check()
	if d.boots != 1 {
		t.Fatalf("boots = %d, want 1", d.boots)
	}
}

func TestWaker_leavesARunningDistroAlone(t *testing.T) {
	now := time.Unix(1000, 0)
	d := &fakeDistro{up: true}
	d.waker(&now).Check()
	if d.boots != 0 {
		t.Errorf("boots = %d, want 0", d.boots)
	}
}

// A page load fires a burst of lookups; only the first one in an interval
// probes the distro, and a failed boot is retried once the interval passes.
func TestWaker_throttlesProbes(t *testing.T) {
	now := time.Unix(1000, 0)
	d := &fakeDistro{}
	w := d.waker(&now)
	w.Boot = func() { d.boots++ } // the boot fails: the distro stays down
	for i := 0; i < 5; i++ {
		w.Check()
	}
	if d.checks != 1 || d.boots != 1 {
		t.Errorf("checks=%d boots=%d within one interval, want 1 and 1", d.checks, d.boots)
	}
	now = now.Add(31 * time.Second)
	w.Check()
	if d.boots != 2 {
		t.Errorf("boots = %d after the interval, want 2", d.boots)
	}
}
