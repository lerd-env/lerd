package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/geodro/lerd/internal/config"
	lerddumps "github.com/geodro/lerd/internal/dumps"
)

// toggleString returns "" when current already equals target (clearing
// the filter) and target otherwise. Used by the chip handlers in the
// dumps view so a second press of `1` or `2` turns the filter off.
func toggleString(current, target string) string {
	if current == target {
		return ""
	}
	return target
}

// dumpsBufferCap is the max number of dump entries the TUI keeps in memory.
// Sized lower than the lerd-ui ring (500) because TUI rendering is per-frame
// and visible viewport is small; older events scroll off the visible list.
const dumpsBufferCap = 200

// debugBatchMsg carries a coalesced batch of raw events (every kind) from the
// SSE listener. Batching caps the TUI's render rate: a busy debug stream (e.g.
// worker capture on a hot queue) can fire hundreds of events a second, and
// rendering once per event would peg the grouped lenses' N+1 grouping. The
// listener flushes on a short ticker instead, so the UI re-renders ~10×/s
// regardless of event throughput. The Dumps lens projects to DumpEntry on
// render; the other lenses read the raw events.
type debugBatchMsg []lerddumps.Event

// debugFlushInterval bounds how often buffered SSE events are handed to the
// program, capping render frequency under a firehose.
const debugFlushInterval = 100 * time.Millisecond

// runDumpsListener opens the lerd-ui Unix-socket SSE endpoint and pumps
// parsed events into the bubbletea program. Reconnects with backoff so the
// TUI keeps refreshing across lerd-ui restarts. Cancelled by ctx.
func runDumpsListener(ctx context.Context, p *tea.Program) {
	backoff := 500 * time.Millisecond
	for ctx.Err() == nil {
		if err := streamDumpsOnce(ctx, p); err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 10*time.Second {
				backoff *= 2
			}
			continue
		}
		// Clean disconnect (lerd-ui exited cleanly): retry sooner.
		backoff = 500 * time.Millisecond
	}
}

// dumpsClientDial reports the transport used to reach the lerd-ui daemon: the
// unix socket on Linux, the TCP loopback on macOS where the socket isn't
// created. A var so tests can point it at a fake listener.
var dumpsClientDial = func() (network, addr string) {
	return config.UIClientNetwork(), config.UIClientAddr()
}

func streamDumpsOnce(ctx context.Context, p *tea.Program) error {
	network, addr := dumpsClientDial()
	conn, err := net.DialTimeout(network, addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://lerd/api/dumps/stream", nil)
	req.Header.Set("Accept", "text/event-stream")
	if err := req.Write(conn); err != nil {
		return err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	// A reader goroutine parses SSE lines into events; the main loop coalesces
	// them and flushes a batch on a short ticker so a high event rate can't
	// drive one render per event.
	events := make(chan lerddumps.Event, 512)
	go func() {
		defer close(events)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), lerddumps.MaxLineBytes)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var ev lerddumps.Event
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
				continue
			}
			select {
			case events <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	flush := time.NewTicker(debugFlushInterval)
	defer flush.Stop()
	var batch []lerddumps.Event
	send := func() {
		if len(batch) > 0 {
			p.Send(debugBatchMsg(batch))
			batch = nil
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				send()
				return nil
			}
			batch = append(batch, ev)
			// Cap a single batch so one render never folds an unbounded burst.
			if len(batch) >= dumpsBufferCap {
				send()
			}
		case <-flush.C:
			send()
		}
	}
}

func toDumpEntry(ev lerddumps.Event) DumpEntry {
	return DumpEntry{
		ID:      ev.ID,
		TS:      ev.TS,
		Type:    ev.Ctx.Type,
		Site:    ev.Ctx.Site,
		Request: ev.Ctx.Request,
		File:    ev.Src.File,
		Line:    ev.Src.Line,
		Label:   ev.Label,
		Text:    ev.Text,
	}
}

// appendDebug folds a new event of any kind into the model, capping the
// buffer at dumpsBufferCap and de-duping on ID (replays from the SSE replay
// path). The cursor is clamped on the next render/nav rather than nudged
// here, since which events are visible depends on the active lens.
func (m *Model) appendDebug(ev lerddumps.Event) {
	for _, existing := range m.debug {
		if existing.ID == ev.ID {
			return
		}
	}
	m.debug = append(m.debug, ev)
	if len(m.debug) > dumpsBufferCap {
		m.debug = m.debug[len(m.debug)-dumpsBufferCap:]
	}
}

// dumpEntries projects the dump-kind events in the buffer to the DumpEntry
// shape the Dumps lens and the per-site Dumps tab render. Non-dump kinds
// (queries, jobs, …) are skipped; each lens reads the raw buffer itself.
func (m *Model) dumpEntries() []DumpEntry {
	out := make([]DumpEntry, 0, len(m.debug))
	for _, ev := range m.debug {
		if ev.Kind == lerddumps.KindDump {
			out = append(out, toDumpEntry(ev))
		}
	}
	return out
}

// filteredDumps returns entries that match needle across any of the
// human-meaningful fields (site, request, label, file, text). Case-
// insensitive; empty needle returns the input unchanged so callers don't
// pay the copy cost. ctx, when set, further restricts to entries whose
// Type matches ("fpm" or "cli") — wired to the chip toggles in the dumps
// header.
func filteredDumps(in []DumpEntry, needle string) []DumpEntry {
	return filteredDumpsWithCtx(in, needle, "")
}

func filteredDumpsWithCtx(in []DumpEntry, needle, ctx string) []DumpEntry {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" && ctx == "" {
		return in
	}
	out := make([]DumpEntry, 0, len(in))
	for _, e := range in {
		if ctx != "" && !strings.EqualFold(e.Type, ctx) {
			continue
		}
		if needle != "" && !dumpMatches(e, needle) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func dumpMatches(e DumpEntry, needle string) bool {
	if strings.Contains(strings.ToLower(e.Site), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Request), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Label), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(e.File), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Text), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Type), needle) {
		return true
	}
	return false
}

// dumpBodyLines returns the lines to render below an entry header: a brief
// preview by default (first 4 lines), or the full text when expanded.
// Wraps long lines at width; expanded entries render every line.
func dumpBodyLines(e DumpEntry, width int, expanded bool) []string {
	if e.Text == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(e.Text, "\n"), "\n")
	if !expanded {
		const maxPreview = 4
		if len(lines) > maxPreview {
			lines = append(lines[:maxPreview], fmt.Sprintf("… (%d more lines · enter to expand)", len(lines)-maxPreview))
		}
	}
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if width > 0 && len(ln) > width {
			ln = ln[:width-1] + "…"
		}
		out = append(out, ln)
	}
	return out
}

// renderDumpsChips formats the FPM / CLI context-filter chips that sit
// under the dumps header. Active chip wears the accent background; the
// inactive one shows in dim text so the user knows the toggle exists.
// The "all" chip is implicit — both off means everything is shown.
func renderDumpsChips(active string) string {
	chip := func(label, key string, on bool) string {
		text := " " + key + " " + label + " "
		if on {
			return keyChipStyle.Render(text)
		}
		return dimStyle.Render("[ " + key + " " + label + " ]")
	}
	return chip("fpm", "1", active == "fpm") + "  " + chip("cli", "2", active == "cli")
}

// dumpsBridgeStateLabel returns a coloured "bridge on/off" label for the
// dumps header. Reads the canonical config every call so the indicator
// updates as soon as the user toggles with T.
func dumpsBridgeStateLabel() string {
	cfg, _ := config.LoadGlobal()
	if cfg != nil && cfg.IsDumpsEnabled() {
		return runningStyle.Render("bridge on")
	}
	return failingStyle.Render("bridge off") + dimStyle.Render(" (T to enable)")
}

// clearDumps stages a confirm modal so a stray `c` keypress doesn't wipe
// buffered events the user is actively reading. On y the local buffer is
// zeroed and `lerd dump clear` runs against the daemon ring. Matches the
// removeFocusedDomain pattern: single-key destructive actions go through
// openConfirm so the policy is consistent across the TUI.
// dumpsClearedMsg tells Update to zero the local dump buffer after the user
// confirms a clear. Applied in Update so the model is never mutated from a
// command goroutine.
type dumpsClearedMsg struct{}

func (m *Model) clearDumps() tea.Cmd {
	count := len(m.debug)
	if count == 0 {
		// Empty buffer — go straight to the daemon clear without a prompt
		// since there's nothing local to lose.
		m.setStatus("cleared dump buffer…", 3*time.Second)
		return runLerd("", "dump", "clear")
	}
	body := fmt.Sprintf("Drop %d buffered events from the dashboard and run `lerd dump clear` against the daemon ring? This cannot be undone.", count)
	// Zeroing the buffer happens in Update via dumpsClearedMsg, never inside
	// this command closure: a tea.Cmd runs on its own goroutine and mutating
	// the model here would race View/Update.
	m.openConfirm("Clear dumps", body, tea.Sequence(
		func() tea.Msg { return dumpsClearedMsg{} },
		runLerd("", "dump", "clear"),
	))
	return nil
}

// toggleDumpsBridge runs `lerd dump on` / `lerd dump off` based on the
// current state. The header label reads the config directly so the change
// is visible on the next refresh tick once the subprocess writes config.
func (m *Model) toggleDumpsBridge() tea.Cmd {
	cfg, _ := config.LoadGlobal()
	enabled := cfg != nil && cfg.IsDumpsEnabled()
	verb := "on"
	if enabled {
		verb = "off"
	}
	m.setStatus("debug bridge "+verb+"…", 5*time.Second)
	return runLerd("", "dump", verb)
}

func dumpHeaderLine(e DumpEntry) string {
	t := shortTime(e.TS)
	parts := []string{t, e.Type}
	if e.Site != "" {
		parts = append(parts, e.Site)
	}
	if e.Request != "" {
		parts = append(parts, e.Request)
	}
	if e.Label != "" {
		parts = append(parts, "$"+e.Label)
	}
	if e.File != "" {
		parts = append(parts, fmt.Sprintf("%s:%d", shortPath(e.File), e.Line))
	}
	return strings.Join(parts, "  ")
}

func dumpPreviewLines(e DumpEntry, width int) []string {
	if e.Text == "" {
		return nil
	}
	const maxPreview = 4
	lines := strings.Split(strings.TrimRight(e.Text, "\n"), "\n")
	if len(lines) > maxPreview {
		lines = append(lines[:maxPreview], fmt.Sprintf("… (%d more lines)", len(lines)-maxPreview))
	}
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if width > 0 && len(ln) > width {
			ln = ln[:width-1] + "…"
		}
		out = append(out, ln)
	}
	return out
}

func shortTime(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		// Fallback: cut to HH:MM:SS portion if present.
		if len(ts) >= 19 && ts[10] == 'T' {
			return ts[11:19]
		}
		return ts
	}
	return t.Local().Format("15:04:05")
}

func shortPath(p string) string {
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	if len(parts) <= 3 {
		return p
	}
	return ".../" + strings.Join(parts[len(parts)-3:], "/")
}
