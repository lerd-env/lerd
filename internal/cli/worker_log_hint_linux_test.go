//go:build linux

package cli

import "testing"

// TestWorkerLogHint_linux pins that the host bool is a no-op on Linux:
// `journalctl --user -u <unit> -f` works for both containerised and
// host-mode workers (host workers are systemd user services that emit
// to the journal like any other unit), so the hint shouldn't fork on
// `host`. The bool is on the signature for cross-platform parity with
// the darwin path that does branch on it.
func TestWorkerLogHint_linux(t *testing.T) {
	const unit = "lerd-vite-acme"
	want := "journalctl --user -u " + unit + " -f"
	if got := workerLogHint(unit, true); got != want {
		t.Errorf("host=true: got %q, want %q", got, want)
	}
	if got := workerLogHint(unit, false); got != want {
		t.Errorf("host=false: got %q, want %q", got, want)
	}
}
