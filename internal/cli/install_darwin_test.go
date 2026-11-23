//go:build darwin

package cli

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPodmanHelperInstalled(t *testing.T) {
	tests := []struct {
		name     string
		plist    string
		username string
		want     bool
	}{
		{"per-user plist", "com.github.containers.podman.helper-george.plist", "george", true},
		{"unsuffixed plist", "com.github.containers.podman.helper.plist", "george", true},
		{"other user's plist", "com.github.containers.podman.helper-alice.plist", "george", false},
		{"not installed", "", "george", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.plist != "" {
				if err := os.WriteFile(filepath.Join(dir, tc.plist), nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if got := podmanHelperInstalled(dir, tc.username); got != tc.want {
				t.Errorf("podmanHelperInstalled = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEnsureFnmBinary_AlreadyPresent is the no-op path used when switching
// back to fnm after fnm was previously installed (or never removed).
func TestEnsureFnmBinary_AlreadyPresent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	binDir := filepath.Join(tmp, "lerd", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	fnmPath := filepath.Join(binDir, "fnm")
	if err := os.WriteFile(fnmPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ensureFnmBinary(io.Discard); err != nil {
		t.Fatalf("ensureFnmBinary: %v", err)
	}
	data, err := os.ReadFile(fnmPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "#!/bin/sh\n" {
		t.Errorf("existing fnm binary was rewritten")
	}
}

// Install only restarts nginx with autostart on, so pausing it for the port
// forward must bring it back itself when autostart is off and it was running.
func TestPauseNginxRestartsWhatInstallWillNot(t *testing.T) {
	prevRunning, prevStop, prevStart, prevAuto := nginxRunningFn, stopNginxFn, startNginxFn, autostartOnFn
	t.Cleanup(func() {
		nginxRunningFn, stopNginxFn, startNginxFn, autostartOnFn = prevRunning, prevStop, prevStart, prevAuto
	})
	cases := []struct {
		running, autostart, wantStart bool
	}{
		{true, false, true},
		{true, true, false},
		{false, false, false},
	}
	for _, tc := range cases {
		stopped, started := false, false
		nginxRunningFn = func() bool { return tc.running }
		stopNginxFn = func() error { stopped = true; return nil }
		startNginxFn = func() error { started = true; return nil }
		autostartOnFn = func() bool { return tc.autostart }
		pauseNginx()()
		if !stopped || started != tc.wantStart {
			t.Errorf("running=%v autostart=%v: stopped=%v started=%v, want started=%v", tc.running, tc.autostart, stopped, started, tc.wantStart)
		}
	}
}
