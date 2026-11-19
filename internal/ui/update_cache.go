package ui

import "sync"

// updateCache holds lerd-ui's answers to "is an update available", so a status
// rebuild every 15 seconds reads them instead of asking podman. Keys are unit
// names (tools excepted); a slot is refilled only after it is dropped.
type updateCache struct {
	mu      sync.Mutex
	entries map[string]any
}

func newUpdateCache() *updateCache { return &updateCache{entries: map[string]any{}} }

// updates is lerd-ui's cache, emptied hourly and when what it holds changes.
var updates = newUpdateCache()

// cachedUpdate returns key's answer, computing it on a miss. The miss blocks,
// so the first status after a restart carries real answers and the notifiers
// do not read every pending update as new.
func cachedUpdate[T any](c *updateCache, key string, compute func() T) T {
	c.mu.Lock()
	if v, ok := c.entries[key].(T); ok {
		c.mu.Unlock()
		return v
	}
	c.mu.Unlock()
	v := compute()
	c.mu.Lock()
	c.entries[key] = v
	c.mu.Unlock()
	return v
}

// dropFor forgets what a change to unit can move, everything when unit is "".
func (c *updateCache) dropFor(unit string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if unit == "" {
		c.entries = map[string]any{}
		return
	}
	delete(c.entries, unit)
}
