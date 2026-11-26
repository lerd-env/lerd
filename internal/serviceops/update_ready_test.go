package serviceops

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func stubRestartAndWait(t *testing.T, waitErr error) *[]string {
	t.Helper()
	withServiceHome(t)
	var calls []string
	prevRestart, prevWait, prevID := imageRestartFn, waitReadyFn, activationIDFn
	imageRestartFn = func(unit string) error { calls = append(calls, "restart "+unit); return nil }
	waitReadyFn = func(name string, _ time.Duration) error { calls = append(calls, "wait "+name); return waitErr }
	activationIDFn = func(string) string { return "run-1" }
	t.Cleanup(func() { imageRestartFn, waitReadyFn, activationIDFn = prevRestart, prevWait, prevID })
	return &calls
}

// A site's first request after an update or rollback must reach an engine that
// accepts connections, not one systemd has only just started.
func TestRestartUntilReady_waitsForTheServiceAfterRestarting(t *testing.T) {
	calls := stubRestartAndWait(t, nil)
	var phases []string
	if err := restartUntilReady("mysql", func(ev PhaseEvent) { phases = append(phases, ev.Phase) }); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*calls, ", "); got != "restart lerd-mysql, wait mysql" {
		t.Errorf("calls = %q, want the restart then the readiness wait", got)
	}
	if got := strings.Join(phases, ","); got != "restarting_unit,waiting_ready" {
		t.Errorf("phases = %q", got)
	}
}

func TestRestartUntilReady_reportsAServiceThatNeverAnswers(t *testing.T) {
	stubRestartAndWait(t, errors.New("mysql did not become ready"))
	err := restartUntilReady("mysql", func(PhaseEvent) {})
	if err == nil || !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("err = %v, want the readiness failure", err)
	}
}
