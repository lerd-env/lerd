package podman

import (
	"bufio"
	"context"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"
)

// eventsResync is how often the cache still polls while the events stream is
// live: only a safety net for an event the stream dropped, as a slept VM can.
const eventsResync = 5 * time.Minute

// stateEvents are the container events that can change whether one is running.
// exec and exec_died are left out on purpose: workers run podman exec all day,
// and each would otherwise cost a podman ps.
var stateEvents = []string{"start", "died", "stop", "pause", "unpause", "remove", "restart"}

// eventsSettle is how long after a burst the cache announces a change again:
// systemd still reads "deactivating" when podman reports died, and a rebuild
// made then would hold that passing state until the snapshot's TTL runs out.
var eventsSettle = 3 * time.Second

func relevantEvent(line string) bool {
	name, status, ok := strings.Cut(strings.TrimSpace(line), "\t")
	if !ok || !strings.HasPrefix(name, "lerd-") {
		return false
	}
	for _, s := range stateEvents {
		if status == s {
			return true
		}
	}
	return false
}

func effectiveInterval(chosen time.Duration, eventsLive bool) time.Duration {
	if eventsLive && chosen < eventsResync {
		return eventsResync
	}
	return chosen
}

// defaultEventsFn opens podman's container event stream, filtered server side
// to the state changes and printed one "name<TAB>status" line per event.
func defaultEventsFn(ctx context.Context) (io.ReadCloser, error) {
	args := []string{"events", "--filter", "type=container", "--format", "{{.Name}}\t{{.Status}}"}
	for _, s := range stateEvents {
		args = append(args, "--filter", "event="+s)
	}
	cmd := CmdContext(ctx, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &cmdReader{ReadCloser: out, cmd: cmd}, nil
}

// cmdReader reaps the podman process when the stream is closed.
type cmdReader struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (r *cmdReader) Close() error {
	err := r.ReadCloser.Close()
	_ = r.cmd.Wait()
	return err
}

// WatchEvents follows the container event stream until ctx ends, refreshing
// the cache on every state change. Where the stream cannot run (journald on
// WSL cannot follow) it retries with backoff while the normal poll carries on.
func (c *ContainerCache) WatchEvents(ctx context.Context) { go c.watchEvents(ctx) }

func (c *ContainerCache) watchEvents(ctx context.Context) {
	fn := c.eventsFn
	if fn == nil {
		fn = defaultEventsFn
	}
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		opened := time.Now()
		if stream, err := fn(ctx); err == nil {
			c.followEvents(stream)
		} else if ctx.Err() == nil {
			log.Printf("[podman] events stream: %v", err)
		}
		if time.Since(opened) > time.Minute {
			backoff = 5 * time.Second // it ran fine for a while; retry soon
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > eventsResync {
			backoff = eventsResync
		}
	}
}

func (c *ContainerCache) followEvents(stream io.ReadCloser) {
	defer stream.Close()
	c.eventsLive.Store(true)
	defer c.eventsLive.Store(false)
	// Events may have been missed while no stream was open.
	c.Refresh()
	var settle *time.Timer
	defer func() {
		if settle != nil {
			settle.Stop()
		}
	}()
	sc := bufio.NewScanner(stream)
	for sc.Scan() {
		if !relevantEvent(sc.Text()) {
			continue
		}
		c.Refresh()
		if settle == nil {
			settle = time.AfterFunc(eventsSettle, c.announceChange)
		} else {
			settle.Reset(eventsSettle)
		}
	}
}

// announceChange runs the change callback without a change in the running map,
// for state that moved underneath it.
func (c *ContainerCache) announceChange() {
	c.onChangeMu.Lock()
	cb := c.onChange
	c.onChangeMu.Unlock()
	if cb != nil {
		cb()
	}
}
