package ui

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// A SIGTERM (lerd stop, launchctl bootout, logout, a systemd restart) must take
// the public tunnels down with the process.
func TestStopTunnelsOnShutdown(t *testing.T) {
	stopped := make(chan struct{})
	prevExit := exit
	exit = func(int) { close(stopped) }
	t.Cleanup(func() { exit = prevExit })

	stopTunnelsOnShutdown()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM did not run the tunnel shutdown")
	}
}
