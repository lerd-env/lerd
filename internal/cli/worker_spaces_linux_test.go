//go:build linux

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// systemd splits ExecStart on whitespace, so a site under a path with a space
// hands podman the second word as the container name: the worker dies with
// `no container with name or ID "Laravel"` on every restart (#893).
func TestWriteWorkerUnitFileQuotesSitePathWithSpaces(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	sitePath := filepath.Join(t.TempDir(), "My Laravel CMS", "spatnik")
	if err := os.MkdirAll(sitePath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	for _, tc := range []struct{ name, schedule, unitName string }{
		{"daemon", "", "lerd-queue-spatnik"},
		{"scheduled", "minutely", "lerd-schedule-spatnik"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := writeWorkerUnitFile(
				tc.unitName, "Queue Worker", "spatnik",
				sitePath, "8.5", "php artisan queue:work",
				"always", tc.schedule, "lerd-php85-fpm", false,
			); err != nil {
				t.Fatalf("writeWorkerUnitFile: %v", err)
			}

			data, err := os.ReadFile(filepath.Join(tmp, "systemd", "user", tc.unitName+".service"))
			if err != nil {
				t.Fatalf("read unit: %v", err)
			}
			unit := string(data)

			if want := "-w '" + sitePath + "'"; !strings.Contains(unit, want) {
				t.Errorf("want %q in:\n%s", want, unit)
			}
			if bare := "-w " + sitePath + " "; strings.Contains(unit, bare) {
				t.Errorf("site path passed unquoted, systemd will split it:\n%s", unit)
			}
		})
	}
}
