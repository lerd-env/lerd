package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// ptyHelperEnv marks the child copy of the test binary a ptySession drives.
const ptyHelperEnv = "LERD_PTY_HELPER"

// ptySession runs one helper test in a child process attached to a pseudo
// terminal, and drives it the way a user would.
//
// Terminal behaviour is otherwise untestable here: go test hands the suite a
// pipe, so every progress view takes its non-TTY branch and the raw mode and
// keypress paths, the ones that break in the field, never run at all.
type ptySession struct {
	t    *testing.T
	f    *os.File
	mu   sync.Mutex
	out  strings.Builder
	done chan struct{}
}

// startPTY launches helper, which must be a test in this package that returns
// early unless ptyHelperEnv is set, and starts draining its terminal.
func startPTY(t *testing.T, helper string) *ptySession {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+helper+"$", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), ptyHelperEnv+"=1")
	f, err := pty.Start(cmd)
	if err != nil {
		t.Skipf("no pseudo terminal available: %v", err)
	}
	s := &ptySession{t: t, f: f, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		buf := make([]byte, 4096)
		for {
			n, readErr := f.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.out.Write(buf[:n])
				s.mu.Unlock()
			}
			if readErr != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		if cmd.Process != nil {
			cmd.Process.Kill() //nolint:errcheck
		}
		cmd.Wait() //nolint:errcheck
		f.Close()  //nolint:errcheck
		<-s.done
	})
	return s
}

// waitFor reports whether text has appeared on the terminal within d.
func (s *ptySession) waitFor(text string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if strings.Contains(s.output(), text) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return strings.Contains(s.output(), text)
}

// send types text at the child, exactly as a user at the keyboard would.
func (s *ptySession) send(text string) {
	s.t.Helper()
	if _, err := s.f.WriteString(text); err != nil {
		s.t.Fatalf("typing %q: %v", text, err)
	}
}

func (s *ptySession) output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.String()
}

// TestPromptAfterProgressViewsGetsItsAnswer is the end-to-end guard for the
// install freeze: a progress view must leave the terminal to whatever reads it
// next, so a question asked afterwards receives what is typed at it. Before the
// fix this hung until the test timeout, with the answer swallowed by readers the
// finished views had left behind.
func TestPromptAfterProgressViewsGetsItsAnswer(t *testing.T) {
	s := startPTY(t, "TestPTYHelperProgressThenPrompt")

	if !s.waitFor("first question", 30*time.Second) {
		t.Fatalf("the prompt never appeared; terminal held:\n%s", s.output())
	}
	s.send("y\n")
	if !s.waitFor("first=true", 15*time.Second) {
		t.Fatalf("the answer typed at the first prompt never reached it; terminal held:\n%s", s.output())
	}

	if !s.waitFor("second question", 15*time.Second) {
		t.Fatalf("the second prompt never appeared; terminal held:\n%s", s.output())
	}
	s.send("y\n")
	if !s.waitFor("second=true", 15*time.Second) {
		t.Fatalf("the answer typed at the second prompt never reached it; terminal held:\n%s", s.output())
	}
}

// TestPTYHelperProgressThenPrompt is not a real test: it is the child that
// TestPromptAfterProgressViewsGetsItsAnswer drives over a terminal. Several
// progress views run before the questions, because a single leftover reader only
// steals the answer, while several also steal the newline that ends it.
func TestPTYHelperProgressThenPrompt(t *testing.T) {
	if os.Getenv(ptyHelperEnv) == "" {
		return
	}
	for i := 0; i < 3; i++ {
		job := BuildJob{Label: fmt.Sprintf("job %d", i), Run: func(io.Writer) error { return nil }}
		RunParallel([]BuildJob{job}) //nolint:errcheck
	}
	fmt.Printf("first=%v\n", confirmInstallPromptDefault("first question", false))
	fmt.Printf("second=%v\n", confirmInstallPromptDefault("second question", false))
}
