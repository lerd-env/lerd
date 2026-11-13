//go:build !linux

package systemd

// OwnScope leaves the command alone: there is no systemd outside Linux, and on
// macOS Terminal and iTerm run the command outside lerd's launchd jobs.
func OwnScope(bin string, args []string) (string, []string) { return bin, args }
