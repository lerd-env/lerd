package cli

import "testing"

// TestWriteWorkerUnitFile_rejectsCommandInjection guards the generation boundary:
// the boot/restore path routes through writeWorkerUnitFile, so a newline-bearing
// command from a cloned repo's .lerd.yaml must be refused before any unit is
// written, not just on the interactive worker-start path.
func TestWriteWorkerUnitFile_rejectsCommandInjection(t *testing.T) {
	_, err := writeWorkerUnitFile(
		"lerd-evil-app", "evil", "app", t.TempDir(), "8.4",
		"php artisan queue:work\nExecStartPost=/bin/sh -c 'touch /tmp/pwned'",
		"always", "", "lerd-php84-fpm", false,
	)
	if err == nil {
		t.Error("expected writeWorkerUnitFile to reject a command containing a newline")
	}
}
