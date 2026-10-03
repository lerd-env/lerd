//go:build !windows

package services

import "errors"

// Supervise only exists on Windows: systemd and launchd restart units on Linux
// and macOS.
func Supervise(_, _ string, _ []string, _ bool) error {
	return errors.New("lerd supervise runs on Windows only")
}
