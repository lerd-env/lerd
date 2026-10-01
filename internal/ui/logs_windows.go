//go:build windows

package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/unitlog"
)

func lerdLogPath(unit string) string { return unitlog.LogPath(unit) }

// isContainerUnit returns true for units whose logs come from `podman logs`
// rather than a file under the data dir.
func isContainerUnit(unit string) bool { return unitlog.IsContainerUnit(unit) }

func serviceRecentLogs(unit string) string {
	if isContainerUnit(unit) {
		out, err := podman.Cmd("logs", "--tail", "20", unit).CombinedOutput()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	f, err := os.Open(lerdLogPath(unit))
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck
	return strings.Join(tailLastLines(f, 20), "\n")
}

// logFollowScript is the command a spawned terminal runs to follow a unit's
// logs: `podman logs -f` for container units, PowerShell's Get-Content -Wait for
// host ones.
func logFollowScript(unit string) string {
	if isContainerUnit(unit) {
		return podman.PodmanBin() + " logs -f --tail 100 " + unit
	}
	return "Get-Content -Wait -Tail 100 -LiteralPath '" + lerdLogPath(unit) + "'"
}

// waitForContainer polls until the named container exists, up to about ten
// seconds, so a stream opened right after a start does not fail.
func waitForContainer(ctx context.Context, unit string) {
	for i := 0; i < 20; i++ {
		if podman.Cmd("container", "exists", unit).Run() == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// streamUnitLogs streams a unit's logs as SSE: `podman logs -f` for container
// units, a follow of the supervised log file for host ones.
func streamUnitLogs(w http.ResponseWriter, r *http.Request, unit string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// An early comment flushes the headers so EventSource fires onopen even
	// for a silent unit.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	streamCtx, streamCancel := context.WithCancel(r.Context())
	defer streamCancel()
	pr, pw := io.Pipe()

	if isContainerUnit(unit) {
		tail := "100"
		if r.Header.Get("Last-Event-ID") != "" {
			tail = "0"
		}
		// Cancellable so a worker-mode migration can end the stream before it
		// races the migration's `podman rm -f`.
		if isFrameworkWorkerUnit(unit) {
			defer logStreams.Register(unit, streamCancel)()
		}
		waitForContainer(streamCtx, unit)
		cmd := exec.CommandContext(streamCtx, podman.PodmanBin(), "logs", "-f", "--tail", tail, unit)
		cmd.Stdout, cmd.Stderr = pw, pw
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(w, "data: error starting logs: %s\n\n", err.Error())
			flusher.Flush()
			return
		}
		go func() { cmd.Wait(); pw.Close() }() //nolint:errcheck
	} else {
		go func() {
			_ = tailFile(streamCtx, lerdLogPath(unit), 100, pw)
			pw.Close()
		}()
	}

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		fmt.Fprintf(w, "data: %s\n\n", scanner.Text())
		flusher.Flush()
	}
}
