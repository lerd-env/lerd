package ui

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/podman"
)

// handlePhpVersionInstallStream is the SSE variant of the legacy POST /install
// endpoint. It writes the quadlet, then runs the FPM image build piping every
// stdout/stderr line through as `event: log` frames before applying the
// xdebug ini and starting the unit. Final frame is always `event: done` with
// `{"ok": bool, "error": "..."}` so the frontend can flip its busy state.
func handlePhpVersionInstallStream(w http.ResponseWriter, r *http.Request, version string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// SSE frames must be serialised across the multiple goroutines that
	// might be writing — the build's stdout reader plus the closing
	// `done` event. ResponseWriter is not safe for concurrent use.
	var mu sync.Mutex
	send := func(event, data string) {
		mu.Lock()
		defer mu.Unlock()
		var b strings.Builder
		b.WriteString("event: ")
		b.WriteString(event)
		b.WriteByte('\n')
		for _, line := range strings.Split(data, "\n") {
			b.WriteString("data: ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
		w.Write([]byte(b.String())) //nolint:errcheck
		flusher.Flush()
	}

	sendErr := func(msg string) {
		send("log", "[lerd] "+msg)
		send("done", fmt.Sprintf(`{"ok":false,"error":%q}`, msg))
	}

	// 1. Quadlet
	send("log", "[lerd] Writing quadlet for PHP "+version+"...")
	if err := podman.WriteFPMQuadlet(version); err != nil {
		sendErr("write quadlet: " + err.Error())
		return
	}

	// 2. Build — pipe output as it lands
	send("log", "[lerd] Building lerd-php"+strings.ReplaceAll(version, ".", "")+"-fpm:local (1–3min)…")

	pr, pw := io.Pipe()
	buildErr := make(chan error, 1)

	// Build goroutine writes podman's stdout+stderr into the pipe.
	go func() {
		defer pw.Close()
		buildErr <- podman.BuildFPMImageTo(version, false, pw)
	}()

	// Stream the pipe line-by-line into SSE log frames.
	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		send("log", scanner.Text())
	}
	if err := <-buildErr; err != nil {
		sendErr("build failed: " + err.Error())
		return
	}

	// 3. Post-build wiring (small, no need to stream)
	send("log", "[lerd] Storing FPM hash + applying xdebug ini…")
	_ = podman.StoreFPMHash()
	_ = podman.EnsureXdebugIni(version)

	send("log", "[lerd] Starting systemd unit lerd-php"+strings.ReplaceAll(version, ".", "")+"-fpm…")
	short := strings.ReplaceAll(version, ".", "")
	unit := "lerd-php" + short + "-fpm"
	if err := podman.StartUnit(unit); err != nil {
		sendErr("start unit: " + err.Error())
		return
	}
	send("log", "[lerd] PHP "+version+" installed and running.")
	send("done", `{"ok":true}`)
}
