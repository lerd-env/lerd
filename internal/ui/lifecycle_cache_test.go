package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/cli"
)

func countContainerPolls(t *testing.T) *int {
	t.Helper()
	n := 0
	orig := pollContainers
	t.Cleanup(func() { pollContainers = orig })
	pollContainers = func() { n++ }
	return &n
}

// With no dashboard open the container cache polls once a minute, so the tray
// went on showing a stopped lerd as running. A start or stop lerd-ui ran itself
// reads the containers back straight away.
func TestStopReadsContainerStateBackAtOnce(t *testing.T) {
	polls := countContainerPolls(t)
	orig := runStop
	t.Cleanup(func() { runStop = orig })
	runStop = func() error { return nil }

	handleLerdStop(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/lerd/stop", nil))

	if *polls != 1 {
		t.Errorf("container cache polled %d times after stop, want 1", *polls)
	}
}

func TestStartReadsContainerStateBackAtOnce(t *testing.T) {
	polls := countContainerPolls(t)
	stubRunStart(t, func(func(cli.StartEvent), ...string) error { return nil })

	postStart(t)

	if *polls != 1 {
		t.Errorf("container cache polled %d times after start, want 1", *polls)
	}
}
