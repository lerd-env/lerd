package mcp

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// Under the native runtime a tool call runs on this machine, and a client that
// closes the session must take a call that never returns down with it.
func TestRunOnHost_endsWhenTheSessionCloses(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	prev := sessionCtx
	sessionCtx = ctx
	t.Cleanup(func() { sessionCtx = prev })

	done := make(chan error, 1)
	go func() {
		var out bytes.Buffer
		done <- runOnHost(t.TempDir(), []string{"sh", "-c", "sleep 30"}, &out)
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the call outlived its session")
	}
}
