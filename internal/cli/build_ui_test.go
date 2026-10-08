package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// Without a terminal (lerd-ui, autostart) the jobs must still run at once: each
// one here waits for the other to have started, which a one-at-a-time runner
// never lets happen. Each job's output must still come out in one piece.
func TestRunWithoutTerminalRunsJobsConcurrentlyWithGroupedOutput(t *testing.T) {
	var started sync.WaitGroup
	started.Add(2)
	job := func(name string) BuildJob {
		return BuildJob{Label: name, Run: func(w io.Writer) error {
			started.Done()
			waited := make(chan struct{})
			go func() { started.Wait(); close(waited) }()
			select {
			case <-waited:
			case <-time.After(5 * time.Second):
				return errors.New("the other job never started alongside this one")
			}
			for i := 1; i <= 3; i++ {
				fmt.Fprintf(w, "%s%d ", name, i)
				time.Sleep(time.Millisecond)
			}
			return nil
		}}
	}

	var out bytes.Buffer
	if err := runWithoutTerminal([]BuildJob{job("a"), job("b")}, &out); err != nil {
		t.Fatalf("runWithoutTerminal: %v", err)
	}
	for _, want := range []string{"a1 a2 a3 ", "b1 b2 b3 "} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q: want %q in one piece", out.String(), want)
		}
	}
}

func TestRunWithoutTerminalReturnsAJobError(t *testing.T) {
	boom := errors.New("boom")
	jobs := []BuildJob{
		{Label: "ok", Run: func(io.Writer) error { return nil }},
		{Label: "bad", Run: func(io.Writer) error { return boom }},
	}
	if err := runWithoutTerminal(jobs, io.Discard); !errors.Is(err, boom) {
		t.Fatalf("want %v, got %v", boom, err)
	}
}
