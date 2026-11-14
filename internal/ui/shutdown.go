package ui

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/plugins"
)

// exit is a seam so the handler can be tested without ending the test binary.
var exit = os.Exit

// cleanUpOnShutdown kills our public tunnels and plugin schedule runs on any
// stop signal. Each runs in its own process group, which macOS has no
// control-group kill for, so without this they outlive lerd-ui.
func cleanUpOnShutdown() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-ch
		cli.StopAllTunnels()
		plugins.StopSchedules()
		exit(0)
	}()
}
