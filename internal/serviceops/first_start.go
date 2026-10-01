package serviceops

import (
	"fmt"
	"os"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
)

// Seams for the first-start recovery, so tests decide readiness and units.
var (
	firstStartReady = podman.WaitReady
	firstStartStop  = podman.StopUnit
	firstStartStart = startUnitRetry
)

// waitReadyFirstStart waits for name to accept connections. An engine stopped
// while it initialised a fresh data directory fails every start after it, so
// while the first start is still pending the directory is moved aside and the
// service started once more on a clean one.
func waitReadyFirstStart(name string, timeout time.Duration) error {
	err := firstStartReady(name, timeout)
	if err == nil {
		config.ClearFirstStartPending(name)
		return nil
	}
	if !config.FirstStartPending(name) {
		return err
	}
	dir := config.DataSubDir(name)
	aside := fmt.Sprintf("%s.incomplete-first-start-%s", dir, time.Now().Format("20060102-150405"))
	unit := "lerd-" + name
	_ = firstStartStop(unit)
	if rerr := os.Rename(dir, aside); rerr != nil {
		return err
	}
	if merr := os.MkdirAll(dir, 0o755); merr != nil {
		return fmt.Errorf("recreating %s: %w", dir, merr)
	}
	feedback.Warn("%s never finished its first start; its unused data directory was moved to %s and it is starting again", name, aside)
	if serr := firstStartStart(unit); serr != nil {
		return serr
	}
	if err := firstStartReady(name, timeout); err != nil {
		return err
	}
	config.ClearFirstStartPending(name)
	return nil
}

// WaitReady is waitReadyFirstStart for callers outside the package, such as
// lerd start's readiness wait.
func WaitReady(name string, timeout time.Duration) error { return waitReadyFirstStart(name, timeout) }
