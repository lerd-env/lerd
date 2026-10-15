package plugins

import (
	"fmt"
	"os/exec"
	"sync/atomic"
	"time"
)

// How long each surface lets a plugin's executable run before stopping it. A
// CLI command has none: it runs in the user's terminal, where Ctrl-C is theirs.
const (
	APILimit         = 30 * time.Second
	MCPLimit         = 2 * time.Minute
	SiteCommandLimit = 30 * time.Minute

	// An entry may ask for longer with timeout:, up to these.
	maxSiteCommandLimit = time.Hour
	maxEntryLimit       = 10 * time.Minute
)

// hookLimit stops a hung hook from holding up the operation that fired it, and
// provideLimit a job lerd handed over; a var so tests need not wait it out.
var (
	hookLimit    = 30 * time.Second
	provideLimit = 5 * time.Minute
)

func (c SiteCommand) Limit() time.Duration { return limitOr(c.Timeout, SiteCommandLimit) }
func (t MCPTool) Limit() time.Duration     { return limitOr(t.Timeout, MCPLimit) }
func (e Endpoint) Limit() time.Duration    { return limitOr(e.Timeout, APILimit) }

func limitOr(raw string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	return def
}

// checkLimit refuses a timeout that is not a positive duration or is past the
// ceiling for its surface.
func checkLimit(raw string, ceiling time.Duration) error {
	if raw == "" {
		return nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("timeout %q is not a duration like 90s or 10m", raw)
	}
	if d <= 0 {
		return fmt.Errorf("timeout %q must be positive", raw)
	}
	if d > ceiling {
		return fmt.Errorf("timeout %s is past the %s limit", d, ceiling)
	}
	return nil
}

// RunWithin runs cmd and stops it once limit passes, every surface but the
// CLI going through it. A script's own children
// can keep its output open after it is killed, so the wait gives up on them
// shortly after rather than hanging on a pipe.
func RunWithin(cmd *exec.Cmd, limit time.Duration) error {
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return err
	}
	var timedOut atomic.Bool
	timer := time.AfterFunc(limit, func() {
		timedOut.Store(true)
		_ = cmd.Process.Kill()
	})
	err := cmd.Wait()
	timer.Stop()
	if timedOut.Load() {
		return fmt.Errorf("timed out after %s", limit)
	}
	return err
}
