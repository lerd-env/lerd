package ui

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

// A SIGTERM (lerd stop, launchctl bootout, logout, a systemd restart) must take
// the public tunnels down with the process.
func TestCleanUpOnShutdown(t *testing.T) {
	stopped := make(chan struct{})
	prevExit := exit
	exit = func(int) { close(stopped) }
	t.Cleanup(func() { exit = prevExit })

	cleanUpOnShutdown()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM did not run the tunnel shutdown")
	}
}

// A restart keeps what Inspect can open: the buffer is written on the way out
// and read back by the next start.
func TestCleanUpOnShutdownSavesTheDebugBuffer(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	srv, err := dumps.Listen(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	srv.Push(dumps.Event{V: dumps.ProtocolVersion, ID: "e1", Kind: dumps.KindQuery, Ctx: dumps.Context{RID: "r1"}})
	prev := dumpsServer.Load()
	dumpsServer.Store(srv)
	t.Cleanup(func() { dumpsServer.Store(prev) })

	stopped := make(chan struct{})
	prevExit := exit
	exit = func(int) { close(stopped) }
	t.Cleanup(func() { exit = prevExit })

	cleanUpOnShutdown()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM did not run the shutdown")
	}

	next := dumps.NewRing(10)
	if err := next.Load(config.DumpsBufferFile()); err != nil || !next.RequestIDs()["r1"] {
		t.Fatalf("restored request ids = %v, err %v", next.RequestIDs(), err)
	}
}
