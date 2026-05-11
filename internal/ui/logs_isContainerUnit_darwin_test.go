//go:build darwin

package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestIsContainerUnit_darwin pins the routing decision logs_darwin's
// SSE stream relies on after the plist-path typo fix + the host
// worker support that landed in this PR. Previous behaviour matched
// `lerd.<unit>.plist` (typo) so the plist branch never fired and the
// framework-prefix fallback masked the bug for built-in workers —
// vite host workers (no framework prefix) routed to podman logs and
// streamed nothing.
func TestIsContainerUnit_darwin(t *testing.T) {
	cases := []struct {
		name  string
		unit  string
		setup func(t *testing.T, home string)
		want  bool
	}{
		{
			name: "service plist (RunAtLoad) -> not a container",
			unit: "lerd-vite-acme",
			setup: func(t *testing.T, home string) {
				writePlist(t, home, "lerd-vite-acme.plist",
					`<plist><dict><key>RunAtLoad</key><true/></dict></plist>`)
			},
			want: false,
		},
		{
			name: "container plist (no RunAtLoad) -> container",
			unit: "lerd-mysql",
			setup: func(t *testing.T, home string) {
				writePlist(t, home, "lerd-mysql.plist",
					`<plist><dict><key>KeepAlive</key><true/></dict></plist>`)
			},
			want: true,
		},
		{
			name: "no plist + guard script present -> not a container (host or exec worker)",
			unit: "lerd-vite-acme",
			setup: func(t *testing.T, home string) {
				dir := filepath.Join(home, "data", "lerd", "run", "workers")
				_ = os.MkdirAll(dir, 0755)
				_ = os.WriteFile(filepath.Join(dir, "lerd-vite-acme.sh"), []byte("#!/bin/sh\n"), 0755)
			},
			want: false,
		},
		{
			name: "no plist + no guard script + framework prefix + exec mode -> not a container",
			unit: "lerd-queue-acme",
			setup: func(t *testing.T, home string) {
				cfg, _ := config.LoadGlobal()
				cfg.Workers.ExecMode = config.WorkerExecModeExec
				_ = config.SaveGlobal(cfg)
			},
			want: false,
		},
		{
			name: "no plist + no guard script + framework prefix + container mode -> container",
			unit: "lerd-queue-acme",
			setup: func(t *testing.T, home string) {
				cfg, _ := config.LoadGlobal()
				cfg.Workers.ExecMode = config.WorkerExecModeContainer
				_ = config.SaveGlobal(cfg)
			},
			want: true,
		},
		{
			name:  "no plist + no guard script + non-framework unit -> container",
			unit:  "lerd-mysql",
			setup: func(t *testing.T, home string) {},
			want:  true,
		},
		{
			name:  "lerd-dns is hardcoded as native",
			unit:  "lerd-dns",
			setup: func(t *testing.T, home string) {},
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("HOME", tmp)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
			t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
			_ = os.MkdirAll(filepath.Join(tmp, "Library", "LaunchAgents"), 0755)

			tc.setup(t, tmp)

			if got := isContainerUnit(tc.unit); got != tc.want {
				t.Errorf("isContainerUnit(%q) = %v, want %v", tc.unit, got, tc.want)
			}
		})
	}
}

func writePlist(t *testing.T, home, name, body string) {
	t.Helper()
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
