package podman

import (
	"bufio"
	"context"
	"fmt"
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
	r := &cmdReader{ReadCloser: out, cmd: cmd}
	cmd.Stderr = &r.stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return r, nil
}

// cmdReader reaps the podman process when the stream is closed, and returns
// why it exited, so a backend that cannot follow events is not a silent retry.
type cmdReader struct {
	io.ReadCloser
	cmd    *exec.Cmd
	stderr tailBuffer
}

func (r *cmdReader) Close() error {
	_ = r.ReadCloser.Close()
	err := r.cmd.Wait()
	if err != nil {
		if msg := strings.TrimSpace(r.stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}

// tailBuffer keeps the last bytes written to it, enough for an error message.
type tailBuffer struct{ b []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if over := len(t.b) - 2048; over > 0 {
		t.b = t.b[over:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return string(t.b) }

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
	logs := streamLogger{logf: log.Printf}
	for ctx.Err() == nil {
		opened := time.Now()
		stream, err := fn(ctx)
		if err == nil {
			err = c.followEvents(stream)
		}
		if ctx.Err() == nil {
			logs.ended(err)
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

func (c *ContainerCache) followEvents(stream io.ReadCloser) (err error) {
	c.eventsLive.Store(true)
	defer func() {
		err = stream.Close()
		c.eventsLive.Store(false)
		// The loop may be waiting out the slow resync; wake it to poll at its
		// normal pace again.
		c.Refresh()
	}()
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
	return nil
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

// streamLogger reports why the events stream ended, once per distinct reason,
// so a podman that cannot follow events says so without filling the log.
type streamLogger struct {
	logf func(format string, args ...any)
	last string
}

func (l *streamLogger) ended(err error) {
	if err == nil {
		return
	}
	if msg := err.Error(); msg != l.last {
		l.last = msg
		l.logf("[podman] events stream ended: %s", msg)
	}
}
