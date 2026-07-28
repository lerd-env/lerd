package ui

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/geodro/lerd/internal/cli"
)

// exit is a seam so the handler can be tested without ending the test binary.
var exit = os.Exit

// stopTunnelsOnShutdown kills the public tunnels this process started when it
// is asked to stop. Every ordinary stop arrives as a signal: `lerd stop`,
// `launchctl bootout`, a logout, a systemd restart. Without this a tunnel is
// left serving the site publicly, because it runs in a process group of its own
// and macOS has no equivalent of the systemd control-group kill.
func stopTunnelsOnShutdown() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-ch
		cli.StopAllTunnels()
		exit(0)
	}()
}
