package feedback

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestAskReadsTheWholeLine(t *testing.T) {
	defer SetStdinInteractive(true)()
	captureOut(t)
	defer setConfirmReader(strings.NewReader("  too heavy for my laptop \nnext\n"))()

	if got := Ask("why?"); got != "too heavy for my laptop" {
		t.Fatalf("Ask = %q", got)
	}
	if got := Ask("and?"); got != "next" {
		t.Fatalf("second Ask = %q, the first read must not swallow the next line", got)
	}
}

func TestAskReturnsEmptyAtEOF(t *testing.T) {
	defer SetStdinInteractive(true)()
	captureOut(t)
	defer setConfirmReader(strings.NewReader(""))()

	if got := Ask("why?"); got != "" {
		t.Fatalf("Ask = %q, want empty", got)
	}
}

func TestAskDoesNotReadWithoutATerminal(t *testing.T) {
	defer SetStdinInteractive(false)()
	captureOut(t)
	blocked, _ := io.Pipe()
	defer setConfirmReader(blocked)()

	done := make(chan string, 1)
	go func() { done <- Ask("why?") }()
	select {
	case got := <-done:
		if got != "" {
			t.Fatalf("Ask = %q, want empty", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask blocked on a stdin that is not a terminal")
	}
}

// A cooked terminal hands arrow keys over as escape sequences, so someone
// pressing down to pick from a list must not have "\x1b[B" sent as their answer.
func TestAskDropsKeysThatAreNotText(t *testing.T) {
	defer SetStdinInteractive(true)()
	captureOut(t)
	defer setConfirmReader(strings.NewReader("\x1b[B\x1b[A\n\x1b[B2\x1bOA\n\x1b[1;5Cslow\x7f laptop\t\ntoo\theavy\n"))()

	for _, want := range []string{"", "2", "slow laptop", "too heavy"} {
		if got := Ask("why?"); got != want {
			t.Fatalf("Ask = %q, want %q", got, want)
		}
	}
}
