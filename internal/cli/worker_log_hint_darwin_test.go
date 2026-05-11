//go:build darwin

package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestWorkerLogHint_darwin pins the host-aware behaviour added when
// host workers became a first-class shape on macOS. The "Logs: …" hint
// the CLI prints after `lerd worker start` must point at the launchd
// log file for host workers in any cfg mode — `podman logs` would
// fail because host workers never live behind a container.
func TestWorkerLogHint_darwin(t *testing.T) {
	cases := []struct {
		name       string
		mode       string
		host       bool
		wantPrefix string
		wantSubstr string
	}{
		{
			name:       "container mode + container worker -> podman logs",
			mode:       config.WorkerExecModeContainer,
			host:       false,
			wantPrefix: "podman logs -f ",
			wantSubstr: "lerd-queue-acme",
		},
		{
			name:       "container mode + host worker -> launchd log file",
			mode:       config.WorkerExecModeContainer,
			host:       true,
			wantPrefix: "tail -f ",
			wantSubstr: filepath.Join("Library", "Logs", "lerd", "lerd-vite-acme.log"),
		},
		{
			name:       "exec mode + container worker -> launchd log file",
			mode:       config.WorkerExecModeExec,
			host:       false,
			wantPrefix: "tail -f ",
			wantSubstr: filepath.Join("Library", "Logs", "lerd", "lerd-queue-acme.log"),
		},
		{
			name:       "exec mode + host worker -> launchd log file",
			mode:       config.WorkerExecModeExec,
			host:       true,
			wantPrefix: "tail -f ",
			wantSubstr: filepath.Join("Library", "Logs", "lerd", "lerd-vite-acme.log"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("HOME", tmp)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
			t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

			cfg, _ := config.LoadGlobal()
			cfg.Workers.ExecMode = tc.mode
			if err := config.SaveGlobal(cfg); err != nil {
				t.Fatal(err)
			}

			unit := "lerd-queue-acme"
			if tc.host {
				unit = "lerd-vite-acme"
			}
			got := workerLogHint(unit, tc.host)
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Errorf("hint = %q, want prefix %q", got, tc.wantPrefix)
			}
			if !strings.Contains(got, tc.wantSubstr) {
				t.Errorf("hint = %q, want substring %q", got, tc.wantSubstr)
			}
		})
	}
}
