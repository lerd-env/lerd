// Package power reports the host's power state so lerd can back off work that
// is cheap on mains and expensive on a laptop running from its battery.
package power

import (
	"sync"
	"time"
)

// State is how the host is currently powered, ordered by how much lerd should
// hold back.
type State int

const (
	// Mains is plugged in with no energy-saving mode engaged.
	Mains State = iota
	// Battery is running from the battery.
	Battery
	// LowPower is an explicit user request to conserve energy (macOS Low Power
	// Mode, the low-power ACPI platform profile). It outranks Battery: the
	// setting can be switched on while plugged in, and when it is, the user has
	// asked for less background work regardless of the power source.
	LowPower
)

func (s State) String() string {
	switch s {
	case LowPower:
		return "low-power"
	case Battery:
		return "battery"
	default:
		return "mains"
	}
}

// probeTTL caches the answer briefly. The probe shells out on macOS and callers
// arrive in bursts (a `lerd start` writes one unit per worker per site), so a
// short window collapses a dozen probes into one without letting the answer go
// stale enough to matter for the cadences that read it.
const probeTTL = 10 * time.Second

// probeFn is the platform detector, swappable in tests.
var probeFn = detectState

var (
	mu       sync.Mutex
	cached   State
	cachedAt time.Time
)

// Current reports how the host is powered. Anything undeterminable reports
// Mains: the conservative answer keeps the normal cadence rather than degrading
// behaviour on a desktop, a VM, or a container that never had a battery.
func Current() State {
	mu.Lock()
	defer mu.Unlock()
	if !cachedAt.IsZero() && time.Since(cachedAt) < probeTTL {
		return cached
	}
	cached = probeFn()
	cachedAt = time.Now()
	return cached
}

// reset clears the cache; used by tests.
func reset() {
	mu.Lock()
	cached = Mains
	cachedAt = time.Time{}
	mu.Unlock()
}
