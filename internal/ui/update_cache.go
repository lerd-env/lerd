package ui

import "sync"

// updateCache holds lerd-ui's answers to "is an update available", so a status
// rebuild every 15 seconds reads them instead of asking podman. Keys are unit
// names (tools excepted); a slot is refilled only after it is dropped.
type updateCache struct {
	mu      sync.Mutex
	entries map[string]any
	// drops counts dropFor calls per key ("" for drop-all), so an answer worked
	// out across a drop of its key is not stored.
	drops map[string]uint64
}

func newUpdateCache() *updateCache {
	return &updateCache{entries: map[string]any{}, drops: map[string]uint64{}}
}

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
	mine, all := c.drops[key], c.drops[""]
	c.mu.Unlock()
	v := compute()
	c.mu.Lock()
	if c.drops[key] == mine && c.drops[""] == all {
		c.entries[key] = v
	}
	c.mu.Unlock()
	return v
}

// dropFor forgets what a change to unit can move, everything when unit is "".
func (c *updateCache) dropFor(unit string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.drops[unit]++
	if unit == "" {
		c.entries = map[string]any{}
		return
	}
	delete(c.entries, unit)
}
