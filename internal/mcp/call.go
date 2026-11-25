package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"sync"

	"github.com/geodro/lerd/internal/execguard"
)

var errCancelled = errors.New("cancelled by the client")

// callTracker holds the call being served, so a client that gives up on it can
// end what it started. Calls run one at a time.
type callTracker struct {
	mu        sync.Mutex
	id        string
	ctx       context.Context
	cancel    context.CancelFunc
	cancelled map[string]bool
	gaveUp    bool
}

var calls = &callTracker{cancelled: map[string]bool{}}

// begin opens the call for request id, false when the client cancelled it
// while it waited behind another one.
func (c *callTracker) begin(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelled[id] {
		delete(c.cancelled, id)
		return false
	}
	c.id, c.gaveUp = id, false
	c.ctx, c.cancel = context.WithCancel(sessionCtx)
	return true
}

// end closes the call and reports whether the client cancelled it. A session
// closing is not that: a one-shot client closes stdin and still reads answers.
func (c *callTracker) end(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id != id || c.cancel == nil {
		return false
	}
	c.cancel()
	c.id, c.ctx, c.cancel = "", nil, nil
	return c.gaveUp
}

func (c *callTracker) context() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx == nil {
		return sessionCtx
	}
	return c.ctx
}

func (c *callTracker) abort(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil && c.id == id {
		c.gaveUp = true
		c.cancel()
		return
	}
	// A late cancel for a finished call leaves its id here, so the set is
	// dropped once it outgrows what could be queued.
	if len(c.cancelled) >= maxQueued {
		clear(c.cancelled)
	}
	c.cancelled[id] = true
}

// noteCancelled ends the call a notifications/cancelled line names. It runs as
// the line is read: the serve loop is inside that very call.
func noteCancelled(line []byte) {
	if !bytes.Contains(line, []byte("notifications/cancelled")) {
		return
	}
	var n struct {
		Method string `json:"method"`
		Params struct {
			RequestID json.RawMessage `json:"requestId"`
		} `json:"params"`
	}
	if json.Unmarshal(line, &n) != nil || n.Method != "notifications/cancelled" || len(n.Params.RequestID) == 0 {
		return
	}
	calls.abort(string(n.Params.RequestID))
}

// runCall runs project code for the current call, in container or on this
// machine when it is empty, and ends the whole process tree when the client
// cancels or the session closes.
func runCall(cmd *exec.Cmd, container string) error {
	ctx := calls.context()
	if ctx.Err() != nil {
		return errCancelled
	}
	err := execguard.Run(ctx, cmd, container)
	if err != nil && ctx.Err() != nil {
		return errCancelled
	}
	return err
}
