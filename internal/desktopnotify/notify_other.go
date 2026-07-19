//go:build !linux

package desktopnotify

// Supported is false off Linux until a platform emitter lands (macOS via
// osascript). Callers fall back to the browser sink.
func Supported() bool { return false }

// Emit is a no-op off Linux.
func Emit(_ Request) (uint32, error) { return 0, nil }
