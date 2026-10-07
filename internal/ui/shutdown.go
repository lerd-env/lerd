package ui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
)

// exit is a seam so the handler can be tested without ending the test binary.
var exit = os.Exit

// cleanUpOnShutdown saves the debug buffer and kills our public tunnels on any
// stop signal. A tunnel runs in its own process group, which macOS has no
// control-group kill for, so without this it stays serving the site publicly.
func cleanUpOnShutdown() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-ch
		cli.StopAllTunnels()
		if srv := dumpsServer.Load(); srv != nil {
			if err := srv.Save(config.DumpsBufferFile()); err != nil {
				fmt.Printf("[WARN] saving debug events: %v\n", err)
			}
		}
		exit(0)
	}()
}
