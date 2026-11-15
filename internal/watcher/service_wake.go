package watcher

import (
	"fmt"
	"sync"

	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/servicewake"
)

// The steps ensureForCaller takes, vars so tests can stand in for the engine
// and the containers.
var (
	wakeSocketWakeAsleep    = func(names []string) error { return idleEng.wakeServicesNow(names) }
	wakeSocketEnsureRunning = serviceops.EnsureServiceRunning
	wakeSocketReady         = serviceops.ServiceReady
)

// startServiceWakeSocket answers lerd processes that ask for a service, so the
// one manager here starts and probes each service once however many ask.
// Best-effort: without the socket each process brings services up itself.
func startServiceWakeSocket() {
	l, err := servicewake.Listen()
	if err != nil {
		fmt.Printf("[WARN] service wake socket: %v\n", err)
		return
	}
	go servicewake.Serve(l, wakeSocketReady, ensureForCaller)
}

// ensureForCaller brings a service up for a lerd process that asked, waking one
// idle-suspend put to sleep through the engine, which restores its sites. A
// failed wake is the answer; requests arriving together share one attempt.
func ensureForCaller(name string) error {
	return ensureCalls.do(name, func() error {
		if serviceAsleep(name) {
			if err := wakeSocketWakeAsleep([]string{name}); err != nil {
				return err
			}
		}
		return wakeSocketEnsureRunning(name)
	})
}

// sharedCalls runs one call per key at a time, handing its result to every
// caller that asked for the same key while it ran.
type sharedCalls struct {
	mu sync.Mutex
	m  map[string]*sharedCall
}

type sharedCall struct {
	done chan struct{}
	err  error
}

var ensureCalls = &sharedCalls{m: map[string]*sharedCall{}}

func (s *sharedCalls) do(key string, fn func() error) error {
	s.mu.Lock()
	if c, ok := s.m[key]; ok {
		s.mu.Unlock()
		<-c.done
		return c.err
	}
	c := &sharedCall{done: make(chan struct{})}
	s.m[key] = c
	s.mu.Unlock()
	c.err = fn()
	s.mu.Lock()
	delete(s.m, key)
	s.mu.Unlock()
	close(c.done)
	return c.err
}
