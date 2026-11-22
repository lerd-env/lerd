//go:build !linux

package systemd

// SeatForeground is Linux-only: macOS has no logind seat to follow.
func SeatForeground() Foreground { return ForegroundUnknown }
