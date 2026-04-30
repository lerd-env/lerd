package ui

import (
	"context"
	"sync"
)

// logStreamRegistry tracks active worker-log SSE streams so a worker-mode
// migration can cancel them all before issuing podman rm calls. Without
// this, `podman logs -f` streams from open log panels race against the
// migration's `podman rm -f` for the same container, jamming the gvproxy
// connection pool and eventually wedging the podman API socket.
type logStreamRegistry struct {
	mu      sync.Mutex
	streams map[string]map[*context.CancelFunc]struct{}
}

func newLogStreamRegistry() *logStreamRegistry {
	return &logStreamRegistry{streams: make(map[string]map[*context.CancelFunc]struct{})}
}

// Register adds cancel under unit and returns a deregister func the caller
// must invoke when the stream ends. The cancel func is stored by pointer
// so multiple streams for the same unit can each unregister independently.
func (r *logStreamRegistry) Register(unit string, cancel context.CancelFunc) func() {
	r.mu.Lock()
	defer r.mu.Unlock()
	cf := cancel
	cfp := &cf
	if r.streams[unit] == nil {
		r.streams[unit] = make(map[*context.CancelFunc]struct{})
	}
	r.streams[unit][cfp] = struct{}{}
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if set, ok := r.streams[unit]; ok {
			delete(set, cfp)
			if len(set) == 0 {
				delete(r.streams, unit)
			}
		}
	}
}

// CancelAllFor cancels every stream registered under any of the named units.
// The cancelled HTTP handler exits, which sends SIGKILL to its `podman logs -f`
// child via exec.CommandContext, releasing the gvproxy connection.
func (r *logStreamRegistry) CancelAllFor(units []string) {
	r.mu.Lock()
	cfs := make([]context.CancelFunc, 0)
	for _, u := range units {
		for cfp := range r.streams[u] {
			cfs = append(cfs, *cfp)
		}
	}
	r.mu.Unlock()
	for _, c := range cfs {
		c()
	}
}

// logStreams is the process-wide registry. Populated by streamUnitLogs on
// macOS; consumed by the worker-mode migration hook in server.go init.
var logStreams = newLogStreamRegistry()
