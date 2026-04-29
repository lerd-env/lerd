package serviceops

import "sync"

// serviceLocks serialises mutating operations (update / migrate / rollback)
// per service so a double-click in the UI, or a CLI invocation racing the
// dashboard, cannot leave the data dir, image config and quadlet pointing at
// inconsistent versions. One mutex per name, lazily created.
var (
	serviceLocksMu sync.Mutex
	serviceLocks   = map[string]*sync.Mutex{}
)

func lockService(name string) func() {
	serviceLocksMu.Lock()
	mu, ok := serviceLocks[name]
	if !ok {
		mu = &sync.Mutex{}
		serviceLocks[name] = mu
	}
	serviceLocksMu.Unlock()
	mu.Lock()
	return mu.Unlock
}
