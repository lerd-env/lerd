// Package activityping is the host-side client for the lerd-watcher's idle
// control socket: it keeps a worked-on site awake under idle-suspend and asks
// the watcher to resume all suspended workers when the feature is switched off.
package activityping

import (
	"net"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// Site records activity for the named site so a terminal php/composer/npm run
// keeps it awake (and wakes it if asleep). No-op on an empty name.
func Site(name string) {
	if name == "" {
		return
	}
	send("activity " + name)
}

// Enable tells the watcher idle-suspend was turned on, so it spins up the engine,
// access feed, and source watcher at once rather than on the next boot.
func Enable() {
	send("enable")
}

// Disable tells the watcher idle-suspend was turned off, so it resumes every
// suspended worker and tears the session down immediately.
func Disable() {
	send("disable")
}

// send writes one best-effort datagram to the control socket and ignores every
// failure. A missing socket fails the dial almost instantly; the write deadline
// caps a hung send so a caller is never held up.
func send(msg string) {
	conn, err := net.Dial("unixgram", config.ControlSocketPath())
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	_, _ = conn.Write([]byte(msg))
}
