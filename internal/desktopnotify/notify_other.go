//go:build !linux

package desktopnotify

import "errors"

// Supported is false off Linux until a platform emitter lands (macOS via
// osascript). Callers fall back to the browser sink.
func Supported() bool { return false }

// Emit is a no-op off Linux.
func Emit(_ Request) (uint32, error) { return 0, nil }

// AppInstalled is false off Linux; callers fall back to the browser.
func AppInstalled() bool { return false }

// OpenApp is unsupported off Linux.
func OpenApp(_ string) error { return errors.New("desktop app not supported on this platform") }
