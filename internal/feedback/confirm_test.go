package feedback

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

// captureOut points the package writer at a buffer for the duration of a test.
func captureOut(t *testing.T) *bytes.Buffer {
	t.Helper()
	prev := out
	buf := &bytes.Buffer{}
	out = buf
	t.Cleanup(func() { out = prev })
	return buf
}

// A confirm on a stdin that is not a terminal must take its default rather than
// block. Piping into lerd (ssh holding stdin open, a CI runner) used to park
// Confirm in a bare Scanln that never returned, wedging `lerd doctor --fix
// --yes` until it was killed.
func TestConfirmTakesDefaultWhenStdinIsNotATerminal(t *testing.T) {
	defer SetStdinInteractive(false)()
	captureOut(t)

	// An open pipe nobody ever writes to: the shape of the real failure. A read
	// here never returns, so this only passes if Confirm does not attempt one.
	blocked, _ := io.Pipe()
	defer setConfirmReader(blocked)()

	for _, defaultYes := range []bool{true, false} {
		done := make(chan bool, 1)
		go func() { done <- Confirm("proceed?", defaultYes) }()

		select {
		case got := <-done:
			if got != defaultYes {
				t.Errorf("Confirm returned %v, want the default %v", got, defaultYes)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("Confirm blocked on a non-terminal stdin (defaultYes=%v)", defaultYes)
		}
	}
}

// The question still has to be printed when it is auto-answered, so a log of a
// non-interactive run shows what was asked and nothing looks silently skipped.
func TestConfirmStillPrintsTheQuestionWhenAutoAnswered(t *testing.T) {
	defer SetStdinInteractive(false)()
	buf := captureOut(t)

	Confirm("reclaim disk space?", false)

	if !strings.Contains(buf.String(), "reclaim disk space?") {
		t.Errorf("question was not printed:\n%s", buf.String())
	}
}

// On a real terminal the answer still comes from the reader, so an explicit
// "n" beats a true default and an empty line falls back to it.
func TestConfirmReadsTheAnswerWhenStdinIsATerminal(t *testing.T) {
	defer SetStdinInteractive(true)()
	captureOut(t)

	cases := []struct {
		answer     string
		defaultYes bool
		want       bool
	}{
		{"n\n", true, false},
		{"y\n", false, true},
		{"\n", true, true},
		{"\n", false, false},
	}
	for _, c := range cases {
		restore := setConfirmReader(strings.NewReader(c.answer))
		got := Confirm("proceed?", c.defaultYes)
		restore()
		if got != c.want {
			t.Errorf("answer %q with defaultYes=%v: got %v, want %v", c.answer, c.defaultYes, got, c.want)
		}
	}
}

// setConfirmReader swaps the reader Confirm reads from, so a test can drive it
// without a real terminal.
func setConfirmReader(r io.Reader) func() {
	prev := confirmReader
	confirmReader = r
	return func() { confirmReader = prev }
}
