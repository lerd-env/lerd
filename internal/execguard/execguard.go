// Package execguard ties the processes lerd starts in a container to the run
// that started them. podman exec leaves them running when its caller dies or
// gives up, so a test run cut short keeps its browser busy for good.
package execguard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// MarkerEnv is set on every guarded exec; children inherit it, so it finds the
// whole tree, Chromium processes that start their own group included.
const MarkerEnv = "LERD_EXEC"

// killScript TERMs every process carrying one of the markers it is given, and
// only when it found some waits and KILLs what is left; a hung Playwright server
// ignores TERM.
const killScript = `pats=$(mktemp) || exit 0
for m in "$@"; do printf '%s\n' "LERD_EXEC=$m"; done > "$pats"
kill_marked() {
	hit=1
	for f in /proc/[0-9]*/environ; do
		tr '\0' '\n' < "$f" 2>/dev/null | grep -qxFf "$pats" || continue
		p=${f#/proc/}
		kill "-$1" "${p%/environ}" 2>/dev/null && hit=0
	done
	return $hit
}
if kill_marked TERM; then
	sleep 2
	kill_marked KILL
fi
rm -f "$pats"`

type record struct {
	Container string `json:"container"`
	Marker    string `json:"marker"`
	PID       int    `json:"pid"`
	Start     string `json:"start"`
	// Ended is set when the exec returned on its own. A child it put in the
	// background, or one Ctrl+C in a terminal did not reach, can outlive it.
	Ended bool `json:"ended,omitempty"`
}

var (
	killAttempts = 3
	killWait     = 5 * time.Second

	dirFn       = func() string { return filepath.Join(config.DataDir(), "exec") }
	killFn      = Kill
	startTimeFn = processStartTime
	raiseFn     = func(sig os.Signal) {
		if p, err := os.FindProcess(os.Getpid()); err == nil {
			_ = p.Signal(sig)
		}
	}
	notifyFn = func() (<-chan os.Signal, func()) {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		return ch, func() { signal.Stop(ch) }
	}
)

// Run runs cmd, a podman exec into container, under a marker of its own: killed
// at once if ctx ends or lerd is interrupted, left to Sweep if anything outlives
// the exec or lerd is killed outright. A cmd that is not a podman exec just runs.
func Run(ctx context.Context, cmd *exec.Cmd, container string) error {
	if len(cmd.Args) < 2 || cmd.Args[1] != "exec" {
		return cmd.Run()
	}
	marker := newMarker()
	cmd.Args = slices.Insert(cmd.Args, 2, "--env", MarkerEnv+"="+marker)

	r := record{Container: container, Marker: marker, PID: os.Getpid(), Start: startTimeFn(os.Getpid())}
	path, recErr := writeRecord(dirFn(), r)
	// The record outlives the run on every path, so whatever the run leaves in
	// the container, however it ended, is the next Sweep's to kill.
	defer func() {
		if recErr == nil {
			r.Ended = true
			_ = writeFileAtomic(path, r)
		}
	}()
	sigs, stop := notifyFn()
	defer stop()
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return stopRun(cmd, done, container, marker)
	case sig := <-sigs:
		err := stopRun(cmd, done, container, marker)
		if recErr == nil {
			r.Ended = true
			_ = writeFileAtomic(path, r)
		}
		// The signal was meant for lerd, which still has to act on it.
		stop()
		raiseFn(sig)
		return err
	}
}

// stopRun kills the run's container side and waits for the exec to return. A
// kill that lands before the command has started inside finds nothing, so it
// is retried, and a podman client that still hangs on is ended locally.
func stopRun(cmd *exec.Cmd, done <-chan error, container, marker string) error {
	for range killAttempts {
		killFn(container, marker)
		select {
		case err := <-done:
			return err
		case <-time.After(killWait):
		}
	}
	_ = cmd.Process.Kill()
	return <-done
}

// Kill ends every process in container that carries one of markers.
func Kill(container string, markers ...string) {
	args := append([]string{"exec", container, "sh", "-c", killScript, "sh"}, markers...)
	_ = podman.Cmd(args...).Run()
}

// Sweep kills what finished runs, and runs whose lerd no longer exists, left in
// their containers, one exec per container, and drops their records. A
// container that is gone took its processes with it.
func Sweep() {
	paths, _ := filepath.Glob(filepath.Join(dirFn(), "*.json"))
	markers := map[string][]string{}
	var swept []string
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var r record
		if json.Unmarshal(b, &r) != nil {
			_ = os.Remove(path)
			continue
		}
		if !r.Ended && ownerAlive(r) {
			continue
		}
		markers[r.Container] = append(markers[r.Container], r.Marker)
		swept = append(swept, path)
	}
	for container, ms := range markers {
		killFn(container, ms...)
	}
	for _, path := range swept {
		_ = os.Remove(path)
	}
}

// ownerAlive compares start times where the platform has them, so a reused pid
// is not taken for the lerd that wrote the record.
func ownerAlive(r record) bool {
	if r.PID <= 0 || syscall.Kill(r.PID, 0) != nil {
		return false
	}
	return r.Start == "" || startTimeFn(r.PID) == r.Start
}

func newMarker() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%d-%s", os.Getpid(), hex.EncodeToString(b))
}

func writeRecord(dir string, r record) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, r.Marker+".json")
	return path, writeFileAtomic(path, r)
}

// writeFileAtomic goes through a rename, so Sweep never reads a record half
// written and drops it as unreadable.
func writeFileAtomic(path string, r record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
