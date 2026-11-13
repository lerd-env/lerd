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
