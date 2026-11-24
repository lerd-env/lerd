package mcp

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func inCall(t *testing.T, id string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if !calls.begin(id) {
		t.Fatalf("call %s was refused", id)
	}
	t.Cleanup(func() { calls.end(id) })
}

func TestRunCall_reportsTheCommandsOwnFailure(t *testing.T) {
	inCall(t, "1")
	err := runCall(exec.Command("sh", "-c", "exit 3"), "")
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Errorf("err = %v, want exit 3", err)
	}
}

func TestRunCall_endsWhenTheClientCancels(t *testing.T) {
	inCall(t, "7")
	done := make(chan error, 1)
	go func() { done <- runCall(exec.Command("sh", "-c", "sleep 30"), "") }()
	time.Sleep(300 * time.Millisecond)
	noteCancelled([]byte(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":7}}`))
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Errorf("err = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the run outlived its cancellation")
	}
}

func TestCalls_skipARequestCancelledWhileQueued(t *testing.T) {
	noteCancelled([]byte(`{"method":"notifications/cancelled","params":{"requestId":"abc"}}`))
	if calls.begin(`"abc"`) {
		t.Error("a request cancelled while queued must not run")
	}
}

func TestCalls_ignoreACancelForAnotherRequest(t *testing.T) {
	inCall(t, "8")
	noteCancelled([]byte(`{"method":"notifications/cancelled","params":{"requestId":9}}`))
	if calls.context().Err() != nil {
		t.Error("another request's cancel ended this call")
	}
	calls.begin("9") // consumes the cancel recorded for it
}

func TestCalls_endWithTheSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	prev := sessionCtx
	sessionCtx = ctx
	t.Cleanup(func() { sessionCtx = prev })
	inCall(t, "10")
	cancel()
	if calls.context().Err() == nil {
		t.Error("the call outlived its session")
	}
	// Dropped answers would leave a one-shot client with nothing.
	if calls.end("10") {
		t.Error("a closed session was taken for a cancelled call")
	}
}

func TestNoteCancelled_ignoresMalformedLines(t *testing.T) {
	inCall(t, "11")
	for _, l := range []string{``, `nope`, `{"method":"notifications/cancelled"}`, `{"method":"tools/call","params":{"requestId":11}}`} {
		noteCancelled([]byte(l))
	}
	if calls.context().Err() != nil {
		t.Error("a malformed line cancelled the call")
	}
}

func TestCalls_endReportsAClientCancel(t *testing.T) {
	inCall(t, "12")
	noteCancelled([]byte(`{"method":"notifications/cancelled","params":{"requestId":12}}`))
	if !calls.end("12") {
		t.Error("the cancelled call would still be answered")
	}
}
