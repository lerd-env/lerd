package cli

import (
	"bytes"
	"testing"

	"github.com/geodro/lerd/internal/feedback"
)

// runEnvLive must save and restore the previous live line rather than nilling
// the global, so it is reentrant. `lerd env` in an unlinked dir links first, and
// linking runs its own setup env step (a nested runEnvLive); if the nested call
// left envLive nil the outer line's Done/Fail would deref nil and crash. Here
// the outer env run is stood in for by a sentinel live line, and the nested run
// errors out fast (unlinked dir, non-interactive) — the only thing asserted is
// that the sentinel survives.
func TestRunEnvLive_RestoresPreviousLive(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	sentinel := feedback.StartLive("outer")
	envLive = sentinel
	t.Cleanup(func() { envLive = nil })

	if err := runEnvLive(nil, nil); err == nil {
		t.Fatal("expected an error running env in an unlinked, non-interactive dir")
	}
	if envLive != sentinel {
		t.Errorf("runEnvLive left envLive = %v, want the previous live line restored", envLive)
	}
}

// On the animated path runEnvLive shows the failure through live.Fail, which
// marks the error as already shown so the top-level handler in main doesn't
// reprint it as a second "Error: …" line. Verify the returned error is flagged
// AlreadyShown, and that a non-nil error still propagates for the non-zero exit.
func TestEnvCmd_AnimatedFailureIsMarkedAlreadyShown(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()
	defer feedback.SetAnimated(true)()
	defer feedback.SetInteractive(true)()

	cmd := NewEnvCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected an error running env in an unlinked, non-interactive dir")
	}
	if !feedback.AlreadyShown(err) {
		t.Error("env animated failure was not marked AlreadyShown; main will reprint the error")
	}
}
