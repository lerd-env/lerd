//go:build darwin

package cli

import (
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

// Install starts nginx only with autostart on, so a nginx paused for the port
// forward has to be remembered; one on its loopback port is never stopped.
func TestPauseNginxStopsOnlyAWideNginxAndRemembersIt(t *testing.T) {
	prevRunning, prevStop, prevWide, prevPaused := nginxRunningFn, stopNginxFn, nginxWideFn, nginxRestartOwed
	t.Cleanup(func() {
		nginxRunningFn, stopNginxFn, nginxWideFn, nginxRestartOwed = prevRunning, prevStop, prevWide, prevPaused
	})
	cases := []struct{ wide, running, wantStop, wantPaused bool }{
		{true, true, true, true},
		{true, false, true, false},
		{false, true, false, false},
	}
	for _, tc := range cases {
		stopped := false
		nginxRestartOwed = false
		nginxWideFn = func() bool { return tc.wide }
		nginxRunningFn = func() bool { return tc.running }
		stopNginxFn = func() error { stopped = true; return nil }
		pauseNginx()
		if stopped != tc.wantStop || nginxRestartOwed != tc.wantPaused {
			t.Errorf("%+v: stopped=%v nginxRestartOwed=%v", tc, stopped, nginxRestartOwed)
		}
	}
}

// A failed migration takes the old rule away too, so a running nginx must be
// restarted onto its wide publish even with autostart off.
func TestNoteNginxRestartWhenThePublishMoved(t *testing.T) {
	prevRunning, prevOwed := nginxRunningFn, nginxRestartOwed
	t.Cleanup(func() { nginxRunningFn, nginxRestartOwed = prevRunning, prevOwed })
	cases := []struct{ moved, running, want bool }{
		{true, true, true},
		{true, false, false},
		{false, true, false},
	}
	for _, tc := range cases {
		nginxRestartOwed = false
		nginxRunningFn = func() bool { return tc.running }
		noteNginxRestart(tc.moved)
		if nginxRestartOwed != tc.want {
			t.Errorf("%+v: nginxRestartOwed=%v", tc, nginxRestartOwed)
		}
	}
}
