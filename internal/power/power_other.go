//go:build !darwin && !linux

package power

// detectState has no implementation on other platforms; reporting Mains keeps
// the normal cadence rather than degrading behaviour blindly.
func detectState() State { return Mains }
