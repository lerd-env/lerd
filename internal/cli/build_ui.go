package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/feedback"
	"golang.org/x/term"
)

// BuildJob is a labeled build task that writes its output to the provided writer.
type BuildJob struct {
	Label string
	Run   func(w io.Writer) error
}

// RunParallel executes all jobs concurrently with a compact spinner UI.
// In a non-TTY environment it falls back to plain sequential output.
// Returns the first non-nil error, or nil if all jobs succeed.
func RunParallel(jobs []BuildJob) error {
	if len(jobs) == 0 {
		return nil
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return runSequential(jobs)
	}
	return runParallelTUI(jobs)
}

func runSequential(jobs []BuildJob) error {
	var firstErr error
	for _, job := range jobs {
		// Non-TTY path: announce the label BEFORE the job runs so it heads the
		// job's streamed output. feedback.Start is non-animated here and prints
		// nothing until OK/Fail, which would leave the label trailing its output.
		feedback.Line(job.Label)
		if err := job.Run(os.Stdout); err != nil {
			feedback.Warn("%s: %v", job.Label, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

type jobState struct {
	label string
	start time.Time
	mu    sync.Mutex
	buf   bytes.Buffer
	end   time.Time
	done  bool
	err   error
}

func (s *jobState) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *jobState) finish(err error) {
	s.mu.Lock()
	s.end = time.Now()
	s.done = true
	s.err = err
	s.mu.Unlock()
}

func (s *jobState) snapshot() (done bool, err error, end time.Time, out []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out = make([]byte, s.buf.Len())
	copy(out, s.buf.Bytes())
	return s.done, s.err, s.end, out
}

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func runParallelTUI(jobs []BuildJob) error {
	states := make([]*jobState, len(jobs))
	for i, job := range jobs {
		states[i] = &jobState{label: job.Label, start: time.Now()}
	}

	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(idx int, j BuildJob) {
			defer wg.Done()
			states[idx].finish(j.Run(states[idx]))
		}(i, job)
	}

	allDone := make(chan struct{})
	go func() { wg.Wait(); close(allDone) }()

	// Ctrl+O toggles output visibility.
	var showOutput atomic.Bool

	// Enter raw terminal mode so we can read single keypresses.
	var restore func()
	if oldState, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		restore = func() { term.Restore(int(os.Stdin.Fd()), oldState) } //nolint:errcheck
	} else {
		restore = func() {}
	}

	// Handle SIGINT / Ctrl+C gracefully.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		restore()
		fmt.Print("\r\n")
		os.Exit(1)
	}()

	// Watch for keypresses through a reader that can be called off, so nothing
	// is left reading the terminal once the view is done. A leftover reader
	// races the next thing to read stdin, be that sudo's password prompt on
	// /dev/tty or an install question, and eats the bytes typed at it.
	keys := startHotkeys(int(os.Stdin.Fd()), func(b byte) {
		switch b {
		case 0x0F: // Ctrl+O — toggle output
			showOutput.Store(!showOutput.Load())
		case 0x03: // Ctrl+C
			restore()
			fmt.Print("\r\n")
			os.Exit(1)
		}
	})
	defer keys.stop()

	termWidth := 120
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		termWidth = w
	}

	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()

	frame := 0
	prevLines := 0

	render := func(final bool) {
		show := showOutput.Load()

		var sb strings.Builder

		// Erase previous render.
		if prevLines > 0 {
			fmt.Fprintf(&sb, "\033[%dA\033[J", prevLines)
		}
		lines := 0
		lw := maxLabelWidth(states)

		for _, s := range states {
			done, err, end, _ := s.snapshot()

			var elapsed time.Duration
			if done {
				elapsed = end.Sub(s.start).Round(time.Second)
			} else {
				elapsed = time.Since(s.start).Round(time.Second)
			}
			var icon string
			switch {
			case done && err != nil:
				icon = feedback.Red("✗")
			case done:
				icon = feedback.Green("✓")
			default:
				icon = feedback.Amber(string(spinnerFrames[frame%len(spinnerFrames)]))
			}

			if elapsed >= time.Second {
				mins := int(elapsed.Minutes())
				secs := int(elapsed.Seconds()) % 60
				fmt.Fprintf(&sb, " %s %-*s  %02d:%02d\r\n", icon, lw, s.label, mins, secs)
			} else {
				fmt.Fprintf(&sb, " %s %s\r\n", icon, s.label)
			}
			lines++
		}

		if !final {
			hint := "Ctrl+O: show output"
			if show {
				hint = "Ctrl+O: hide output"
			}
			fmt.Fprintf(&sb, "  %s\r\n", feedback.Dim(hint))
			lines++
		}

		if show {
			for _, s := range states {
				_, _, _, out := s.snapshot()
				if len(out) == 0 {
					continue
				}
				fmt.Fprintf(&sb, "  %s\r\n", feedback.Dim("─── "+s.label+" ───"))
				lines++
				for _, l := range strings.Split(tailLines(out, 20, termWidth-4), "\n") {
					fmt.Fprintf(&sb, "  %s\r\n", l)
					lines++
				}
			}
		}

		prevLines = lines
		fmt.Print(sb.String())
		frame++
	}

	for {
		select {
		case <-tick.C:
			render(false)
		case <-allDone:
			render(true)
			restore()
			signal.Stop(sigCh)
			fmt.Print("\r\n")

			var firstErr error
			for _, s := range states {
				done, err, _, out := s.snapshot()
				if done && err != nil {
					if firstErr == nil {
						firstErr = err
					}
					fmt.Printf("%s %v\n", feedback.Red("✗ "+s.label+":"), err)
					if len(out) > 0 {
						fmt.Printf("  %s\n%s\n", feedback.Dim("─── output ───"), out)
					}
				}
			}
			return firstErr
		}
	}
}

// maxLabelWidth returns the longest label across states (rune count), capped so
// one very long label can't push the timing column off the right edge. Used to
// align the timing column without over-padding short labels into a ragged run
// of trailing whitespace.
func maxLabelWidth(states []*jobState) int {
	w := 0
	for _, s := range states {
		if n := len([]rune(s.label)); n > w {
			w = n
		}
	}
	if w > 40 {
		w = 40
	}
	return w
}

// tailLines returns the last n lines of b, truncated to maxWidth chars each.
func tailLines(b []byte, n, maxWidth int) string {
	clean := strings.ReplaceAll(string(b), "\r", "")
	all := strings.Split(strings.TrimRight(clean, "\n"), "\n")
	if len(all) > n {
		all = all[len(all)-n:]
	}
	for i, l := range all {
		if len(l) > maxWidth {
			all[i] = l[:maxWidth-3] + "..."
		}
	}
	return strings.Join(all, "\n")
}
