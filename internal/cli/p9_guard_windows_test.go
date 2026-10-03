//go:build windows

package cli

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"
)

// fakeP9Guard drives p9Guard.run with servers that exit with codes in turn.
type fakeP9Guard struct {
	codes     []int
	starts    int
	recovers  int
	slept     []time.Duration
	machineUp bool
	startErr  error
}

func (f *fakeP9Guard) guard() p9Guard {
	return p9Guard{
		start: func() (func() int, error) {
			f.starts++
			if f.startErr != nil && f.starts == 1 {
				return nil, f.startErr
			}
			code := f.codes[0]
			f.codes = f.codes[1:]
			return func() int { return code }, nil
		},
		machineUp: func() bool { return f.machineUp },
		recover:   func() error { f.recovers++; return nil },
		sleep:     func(d time.Duration) { f.slept = append(f.slept, d) },
		log:       &bytes.Buffer{},
	}
}

func TestP9GuardRestartsACrashedServerAndRemounts(t *testing.T) {
	f := &fakeP9Guard{codes: []int{1, 2, 0}, machineUp: true}
	f.guard().run()

	if f.starts != 3 {
		t.Errorf("server started %d times, want 3", f.starts)
	}
	// The first start is the takeover's, which remounts itself.
	if f.recovers != 2 {
		t.Errorf("remounted %d times, want once per restart (2)", f.recovers)
	}
	if want := []time.Duration{time.Second, 2 * time.Second}; !reflect.DeepEqual(f.slept, want) {
		t.Errorf("waits = %v, want %v", f.slept, want)
	}
}

func TestP9GuardStopsWithTheMachine(t *testing.T) {
	f := &fakeP9Guard{codes: []int{1}, machineUp: false}
	f.guard().run()

	if f.starts != 1 || f.recovers != 0 || len(f.slept) != 0 {
		t.Errorf("a server that died with its machine should not come back: starts=%d recovers=%d waits=%v", f.starts, f.recovers, f.slept)
	}
}

func TestP9GuardRetriesAFailedStart(t *testing.T) {
	f := &fakeP9Guard{codes: []int{0}, machineUp: true, startErr: errors.New("hvsock busy")}
	f.guard().run()

	if f.starts != 2 {
		t.Errorf("server started %d times, want a retry after the failed start", f.starts)
	}
	if f.recovers != 1 {
		t.Errorf("remounted %d times, want once for the restarted server", f.recovers)
	}
}

func TestIsLerdP9ServeCountsTheGuard(t *testing.T) {
	if !isLerdP9Serve(`C:\lerd\bin\lerd.exe p9-guard --machine m -- --serve C:\:guid 12`) {
		t.Error("a running guard keeps the server up and should count as lerd's")
	}
}
