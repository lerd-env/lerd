package feedback

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	barWidth = 24
	barFull  = "█"
	barEmpty = "░"
)

// renderBar draws a fixed-width bar for done out of total. A zero total reads
// as finished rather than dividing by zero, and overshooting the total stops at
// full rather than running off the end of the line.
func renderBar(done, total int) string {
	filled := barWidth
	if total > 0 {
		filled = done * barWidth / total
		if filled > barWidth {
			filled = barWidth
		}
		if filled < 0 {
			filled = 0
		}
	}
	return strings.Repeat(barFull, filled) + strings.Repeat(barEmpty, barWidth-filled)
}

// Progress is a live line for work of a known size: a bar, a count, and the
// item currently in flight, e.g.
//
//	→ linking 50 projects… ████████░░░░░░░░ 24/50 · proj24 ⠙
//
// It is safe to call from several goroutines. On a non-animated output there is
// no line to redraw, so each item prints on its own instead.
type Progress struct {
	msg      string
	total    int
	animated bool

	done    atomic.Int64
	skipped atomic.Int64
	failed  atomic.Int64

	cmu     sync.Mutex
	current string

	paused bool // guarded by the package mu, like Live
	prev   *activeBox
	stop   chan struct{}
	wg     sync.WaitGroup
}

// StartProgress begins a progress line over total items.
func StartProgress(msg string, total int) *Progress {
	p := &Progress{msg: msg, total: total, animated: Animated()}
	if !p.animated {
		mu.Lock()
		fmt.Fprintf(target(), "%s%s %s\n", pad, "→", msg+"…")
		mu.Unlock()
		return p
	}
	p.prev = pushActive(p)
	p.stop = make(chan struct{})
	p.wg.Add(1)
	go p.spin()
	return p
}

// Step records one completed item.
func (p *Progress) Step(item string) {
	p.done.Add(1)
	p.record(item, "")
}

// Skip records an item that was deliberately not done, with the reason.
func (p *Progress) Skip(item, reason string) {
	p.skipped.Add(1)
	p.record(item, reason)
}

// Failed records an item that could not be done, with the reason.
func (p *Progress) Failed(item, reason string) {
	p.failed.Add(1)
	p.record(item, reason)
}

func (p *Progress) record(item, note string) {
	if !p.animated {
		line := "  " + item
		if note != "" {
			line += " — " + note
		}
		mu.Lock()
		fmt.Fprintf(target(), "%s%s\n", pad, line)
		mu.Unlock()
		return
	}
	p.cmu.Lock()
	p.current = item
	p.cmu.Unlock()
}

// Completed, Skipped and Failures report the tallies, for the caller's summary.
func (p *Progress) Completed() int { return int(p.done.Load()) }
func (p *Progress) Skipped() int   { return int(p.skipped.Load()) }
func (p *Progress) Failures() int  { return int(p.failed.Load()) }

// Interrupt suspends the line so fn can print standalone output above it, the
// same contract as Live.Interrupt.
func (p *Progress) Interrupt(fn func()) {
	if !p.animated {
		fn()
		return
	}
	mu.Lock()
	wasPaused := p.paused
	p.paused = true
	if !wasPaused {
		fmt.Fprint(target(), "\r\033[2K")
	}
	mu.Unlock()
	fn()
	mu.Lock()
	p.paused = wasPaused
	mu.Unlock()
}

// Done stops the line and leaves a ✓ with the caller's summary.
func (p *Progress) Done(summary string) {
	if !p.animated {
		mu.Lock()
		fmt.Fprintf(target(), "%s%s %s %s\n", pad, paint(okStyle, "✓"), p.msg, paint(dimStyle, summary))
		mu.Unlock()
		return
	}
	close(p.stop)
	p.wg.Wait()
	popActive(p.prev)
	mu.Lock()
	fmt.Fprintf(target(), "\r\033[2K%s%s %s %s\n", pad, paint(okStyle, "✓"), p.msg, paint(dimStyle, summary))
	mu.Unlock()
}

func (p *Progress) spin() {
	defer p.wg.Done()
	t := time.NewTicker(90 * time.Millisecond)
	defer t.Stop()
	i := 0
	p.draw(spinnerFrames[0])
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			i++
			p.draw(spinnerFrames[i%len(spinnerFrames)])
		}
	}
}

func (p *Progress) draw(frame string) {
	settled := p.Completed() + p.Skipped() + p.Failures()
	p.cmu.Lock()
	current := p.current
	p.cmu.Unlock()

	mu.Lock()
	defer mu.Unlock()
	if p.paused {
		return
	}
	line := pad + paint(dimStyle, "→") + " " + paint(dimStyle, p.msg+"…") +
		" " + paint(okStyle, renderBar(settled, p.total)) +
		" " + paint(valueStyle, fmt.Sprintf("%d/%d", settled, p.total))
	if current != "" {
		line += " " + paint(dimStyle, "· "+current)
	}
	if frame != "" {
		line += " " + paint(spinStyle, frame)
	}
	fmt.Fprintf(target(), "\r\033[2K%s", line)
}
