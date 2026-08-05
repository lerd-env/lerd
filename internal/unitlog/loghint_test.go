package unitlog

import (
	"runtime"
	"strings"
	"testing"
)

// A hint is shown to a user who is about to type it, so it has to name a command
// that exists on the platform they are on. journalctl is not one on macOS.
func TestLogHint_namesACommandThisPlatformHas(t *testing.T) {
	hint := LogHint("lerd-myapp-frankenphp")
	if hint == "" {
		t.Fatal("LogHint returned nothing")
	}
	if !strings.Contains(hint, "lerd-myapp-frankenphp") {
		t.Errorf("LogHint = %q, want it to name the unit", hint)
	}
	switch runtime.GOOS {
	case "darwin":
		if strings.Contains(hint, "journalctl") {
			t.Errorf("LogHint = %q, but journalctl does not exist on macOS", hint)
		}
		if !strings.Contains(hint, "Library/Logs/lerd") {
			t.Errorf("LogHint = %q, want it to point at the launchd log file", hint)
		}
	case "linux":
		if !strings.Contains(hint, "journalctl") {
			t.Errorf("LogHint = %q, want the journal command", hint)
		}
	}
}
