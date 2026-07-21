package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The flag has to be interpolatable straight into the exec command, so it
// carries its own leading space and stays empty when nothing needs setting.
func TestWorkerExecEnvFlags(t *testing.T) {
	dir := t.TempDir()
	got := workerExecEnvFlags(dir)

	if !config.WatcherNeedsPolling(dir) {
		if got != "" {
			t.Fatalf("host does not poll, want no env flags, got %q", got)
		}
		return
	}

	if !strings.HasPrefix(got, " --env=CHOKIDAR_INTERVAL=") {
		t.Fatalf("polling host must pass the interval to the container, got %q", got)
	}
	if strings.Contains(strings.TrimPrefix(got, " "), " ") {
		t.Errorf("flag must be a single argument, got %q", got)
	}
}
