// Package feedback renders lerd's CLI progress output (animated steps, a live
// line, summaries, prompts, warnings) and owns the shared lerd colour palette.
// It degrades to plain text when stdout is piped, redirected, or NO_COLOR is set.
package feedback

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Canonical lerd palette (Tailwind hex values). The TUI theme aliases these so
// CLI feedback and the dashboard share one set of colours.
var (
	ColTitle   = lipgloss.Color("#FF2D20") // lerd red (Laravel brand)
	ColDim     = lipgloss.Color("#6b7280") // gray-500
	ColDivider = lipgloss.Color("#374151") // gray-700
	ColRunning = lipgloss.Color("#10b981") // emerald-500
	ColStopped = lipgloss.Color("#6b7280") // gray-500
	ColFailing = lipgloss.Color("#ef4444") // red-500
	ColPaused  = lipgloss.Color("#f59e0b") // amber-400
	// Interactive accent is the brand red so the TUI, CLI feedback, and the web
	// UI all share one accent rather than diverging on a separate hue.
	ColAccent = lipgloss.Color("#FF2D20") // lerd red
)

var (
	dimStyle    = lipgloss.NewStyle().Foreground(ColDim)
	okStyle     = lipgloss.NewStyle().Foreground(ColRunning)
	failStyle   = lipgloss.NewStyle().Foreground(ColFailing).Bold(true)
	valueStyle  = lipgloss.NewStyle().Foreground(ColAccent)
	promptStyle = lipgloss.NewStyle().Foreground(ColAccent).Bold(true)
	spinStyle   = lipgloss.NewStyle().Foreground(ColAccent)
	warnStyle   = lipgloss.NewStyle().Foreground(ColPaused)
	redStyle    = lipgloss.NewStyle().Foreground(ColFailing)
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(ColTitle)
)

// Status glyphs for query/report output that renders its own layout rather than
// using the Step/Live progress flow.
const (
	GlyphOK   = "✓"
	GlyphFail = "✗"
	GlyphWarn = "⚠"
)

// Title styles a fragment as the bold lerd-red wordmark colour.
func Title(s string) string { return paint(titleStyle, s) }

// Green, Red, Amber, and Dim colour a one-off fragment for status reports,
// honouring NO_COLOR and non-TTY output. Use these so query commands share the
// progress palette without reaching for raw ANSI codes.
func Green(s string) string { return paint(okStyle, s) }
func Red(s string) string   { return paint(redStyle, s) }
func Amber(s string) string { return paint(warnStyle, s) }
func Dim(s string) string   { return paint(dimStyle, s) }

// spinnerFrames is the Braille spinner used by the Live progress line.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// pad is the left margin printed before every feedback glyph line so the block
// sits slightly indented from the shell prompt.
const pad = " "

var (
	mu  sync.Mutex
	out io.Writer // nil → write to the current os.Stdout (so redirects are honoured)
	// colorOn is read on every paint() (including from the Live spinner
	// goroutine) and flipped by SetTestWriter, so it's atomic rather than a
	// plain bool guarded only on the write side.
	colorOn atomic.Bool
)

func init() { colorOn.Store(detectColor()) }

// target returns the writer to render to: an explicit test writer when set,
// otherwise the live os.Stdout (looked up per call so os.Stdout redirection,
// e.g. in tests, is respected).
func target() io.Writer {
	if out != nil {
		return out
	}
	return os.Stdout
}

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// paint applies a style only when colour is enabled, so plain-mode output (and
// tests) stay free of escape codes.
func paint(st lipgloss.Style, s string) string {
	if !colorOn.Load() {
		return s
	}
	return st.Render(s)
}

// Val styles a value fragment (violet) for embedding inside a step or summary.
func Val(s string) string { return paint(valueStyle, s) }

// Begin prints a single blank line to separate a feedback block from whatever
// (a shell prompt, a wizard) came before it.
func Begin() {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintln(target())
}

// Step is one line of progress: Start it before the work, then OK/Info/Fail
// finishes it. On a colour TTY a spinner animates until then; otherwise nothing
// prints until the finishing call writes the line once.
type Step struct {
	msg  string
	stop chan struct{}
	wg   sync.WaitGroup
}

// Start records a step with the given present-tense label (e.g. "detecting
// framework") and begins its spinner on an animated terminal.
func Start(msg string) *Step {
	s := &Step{msg: msg}
	if Animated() {
		s.stop = make(chan struct{})
		s.wg.Add(1)
		go s.spin()
	}
	return s
}

func (s *Step) spin() {
	defer s.wg.Done()
	t := time.NewTicker(90 * time.Millisecond)
	defer t.Stop()
	i := 0
	s.frame(spinnerFrames[0])
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			i++
			s.frame(spinnerFrames[i%len(spinnerFrames)])
		}
	}
}

func (s *Step) frame(f string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(target(), "\r\033[2K%s%s %s %s", pad, paint(dimStyle, "→"), paint(dimStyle, s.msg), paint(spinStyle, f))
}

func (s *Step) finish(mark, result string) {
	if Animated() {
		close(s.stop)
		s.wg.Wait()
	}
	mu.Lock()
	defer mu.Unlock()
	line := fmt.Sprintf("%s%s %s", pad, paint(dimStyle, "→"), paint(dimStyle, s.msg+"…"))
	if mark != "" {
		line += " " + mark
	}
	if result != "" {
		line += " " + result
	}
	if Animated() {
		fmt.Fprintf(target(), "\r\033[2K%s\n", line)
	} else {
		fmt.Fprintln(target(), line)
	}
}

// OK collapses the step with a green check and a result (e.g. "Laravel 11").
func (s *Step) OK(result string) { s.finish(paint(okStyle, "✓"), result) }

// Info collapses the step with a plain result and no mark.
func (s *Step) Info(result string) { s.finish("", result) }

// Fail collapses the step with a red cross and the error text.
func (s *Step) Fail(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	s.finish(paint(failStyle, "✗"), paint(failStyle, msg))
}

// Line prints a standalone step (arrow prefix, no trailing ellipsis) for an
// already-known fact, e.g. "php 8.4 · node 22 · nginx vhost written".
func Line(msg string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(target(), "%s%s %s\n", pad, paint(dimStyle, "→"), msg)
}

// Note prints a dim, indented sub-detail under a step (e.g. a log hint). It has
// no glyph so it reads as secondary to the step lines above it.
func Note(msg string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(target(), "%s  %s\n", pad, paint(dimStyle, msg))
}

// Warn prints a warning line: an amber warning glyph and amber message. Use in
// place of a plain "[WARN] …" print.
func Warn(format string, a ...any) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(target(), "%s%s %s\n", pad, paint(warnStyle, "⚠"), paint(warnStyle, fmt.Sprintf(format, a...)))
}

// Done prints a green-check completion line with no timing, for operations
// where elapsed time isn't meaningful.
func Done(msg string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(target(), "%s%s %s\n", pad, paint(okStyle, "✓"), msg)
}

// Success prints the terminal line: green check, message, dim elapsed time.
func Success(msg string, d time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(target(), "%s%s %s %s\n", pad, paint(okStyle, "✓"), msg, paint(dimStyle, "in "+humanDur(d)))
}

// Confirm prints a styled yes/no prompt (preceded by a blank line) and reads
// the answer from stdin, returning defaultYes on an empty response. The prompt
// matches the step styling: a violet "?" lead-in and a dim "[Y/n]" hint.
func Confirm(question string, defaultYes bool) bool {
	hint := "[Y/n]"
	if !defaultYes {
		hint = "[y/N]"
	}
	mu.Lock()
	fmt.Fprintf(target(), "\n%s%s %s %s ", pad, paint(promptStyle, "?"), question, paint(dimStyle, hint))
	mu.Unlock()

	var answer string
	fmt.Scanln(&answer) //nolint:errcheck
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" {
		return defaultYes
	}
	return answer[0] == 'y'
}

// Animated reports whether output supports in-place animation (a colour TTY).
// When false, Live degrades to a header line plus one plain line per item.
func Animated() bool { return colorOn.Load() }

// Live is a single in-place progress line with a trailing spinner that
// accumulates completed items, e.g. "→ configuring .env… ✓ a · b · c ⠙". When
// it finishes the spinner is dropped and the ✓ line stays put.
type Live struct {
	msg    string
	imu    sync.Mutex
	items  []string
	paused bool // guarded by the package mu; set by Interrupt
	stop   chan struct{}
	wg     sync.WaitGroup
}

// StartLive begins a live line with the given label. On a non-animated output
// it just prints the header and each Add on its own line.
func StartLive(msg string) *Live {
	l := &Live{msg: msg}
	if Animated() {
		l.stop = make(chan struct{})
		l.wg.Add(1)
		go l.spin()
	} else {
		mu.Lock()
		fmt.Fprintf(target(), "%s%s %s\n", pad, "→", msg+"…")
		mu.Unlock()
	}
	return l
}

func (l *Live) snapshot() []string {
	l.imu.Lock()
	defer l.imu.Unlock()
	return append([]string(nil), l.items...)
}

func (l *Live) spin() {
	defer l.wg.Done()
	t := time.NewTicker(90 * time.Millisecond)
	defer t.Stop()
	i := 0
	l.draw(spinnerFrames[0])
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			i++
			l.draw(spinnerFrames[i%len(spinnerFrames)])
		}
	}
}

func (l *Live) draw(frame string) {
	items := l.snapshot()
	mu.Lock()
	defer mu.Unlock()
	if l.paused {
		return
	}
	line := pad + paint(dimStyle, "→") + " " + paint(dimStyle, l.msg+"…")
	if len(items) > 0 {
		line += " " + paint(okStyle, "✓") + " " + strings.Join(items, " · ")
	}
	if frame != "" {
		line += " " + paint(spinStyle, frame)
	}
	fmt.Fprintf(target(), "\r\033[2K%s", line)
}

// Interrupt suspends the spinner and clears its line so fn can print standalone
// output (a booting service, a child process's stdout) above the live line
// without the spinner's \r redraw clobbering it, then lets the spinner resume
// and redraw beneath. On non-animated output it just runs fn. paused is read
// and written under the package mu, the same lock draw holds while writing, so
// no redraw can slip between the clear and fn's output.
func (l *Live) Interrupt(fn func()) {
	if !Animated() {
		fn()
		return
	}
	mu.Lock()
	l.paused = true
	fmt.Fprint(target(), "\r\033[2K")
	mu.Unlock()
	fn()
	mu.Lock()
	l.paused = false
	mu.Unlock()
}

// Add records a completed item; the spinning line redraws to include it.
func (l *Live) Add(item string) {
	if !Animated() {
		mu.Lock()
		fmt.Fprintf(target(), "%s  %s\n", pad, item)
		mu.Unlock()
		return
	}
	l.imu.Lock()
	l.items = append(l.items, item)
	l.imu.Unlock()
}

// Done stops the spinner, leaving the ✓ line (checkbox replaces the loader).
func (l *Live) Done() {
	if !Animated() {
		mu.Lock()
		fmt.Fprintf(target(), "%s%s %s\n", pad, paint(okStyle, "✓"), l.msg)
		mu.Unlock()
		return
	}
	close(l.stop)
	l.wg.Wait()
	l.draw("")
	mu.Lock()
	fmt.Fprintln(target())
	mu.Unlock()
}

// Fail stops the spinner and finalises the line with a red cross.
func (l *Live) Fail(err error) {
	if Animated() {
		close(l.stop)
		l.wg.Wait()
	}
	mu.Lock()
	defer mu.Unlock()
	if Animated() {
		fmt.Fprint(target(), "\r\033[2K")
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	fmt.Fprintf(target(), "%s%s %s\n", pad, paint(failStyle, "✗"), paint(failStyle, msg))
}

// Summary is an aligned key/value block printed after an operation completes.
type Summary struct{ rows [][2]string }

// NewSummary returns an empty summary block.
func NewSummary() *Summary { return &Summary{} }

// Row appends a label/value pair; value may contain styled fragments via Val.
func (s *Summary) Row(label, value string) *Summary {
	s.rows = append(s.rows, [2]string{label, value})
	return s
}

// Print renders the block, label column padded to the widest label, preceded by
// a blank line. No-op when empty.
func (s *Summary) Print() {
	if len(s.rows) == 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	w := 0
	for _, r := range s.rows {
		if len(r[0]) > w {
			w = len(r[0])
		}
	}
	fmt.Fprintln(target())
	for _, r := range s.rows {
		label := r[0] + strings.Repeat(" ", w-len(r[0]))
		fmt.Fprintf(target(), "  %s   %s\n", paint(dimStyle, label), r[1])
	}
}

// SetTestWriter redirects output to w in plain mode (no colour) and returns a
// restore func. Intended for tests in this and other packages.
func SetTestWriter(w io.Writer) func() {
	mu.Lock()
	prevOut, prevColor := out, colorOn.Load()
	out = w
	colorOn.Store(false)
	mu.Unlock()
	return func() {
		mu.Lock()
		out = prevOut
		colorOn.Store(prevColor)
		mu.Unlock()
	}
}

func humanDur(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
