//go:build !linux

package execguard

// processStartTime has no cheap source off Linux, so liveness falls back to the
// pid alone.
func processStartTime(int) string { return "" }
