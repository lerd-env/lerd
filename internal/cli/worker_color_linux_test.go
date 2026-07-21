//go:build linux

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Linux units are the path every Linux install actually gets, so pin the
// colour environment on the real generated unit bodies rather than trusting the
// shared builders alone.
func TestWriteWorkerUnitFileForcesColour(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	sitePath := t.TempDir()
	unitPath := func(name string) string {
		return filepath.Join(tmp, "systemd", "user", name+".service")
	}

	for _, tc := range []struct{ name, schedule, unitName string }{
		{"daemon", "", "lerd-queue-alpha"},
		{"scheduled", "minutely", "lerd-schedule-alpha"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := writeWorkerUnitFile(
				tc.unitName, "Queue Worker", "alpha", sitePath, "8.4",
				"php artisan queue:work", "always", tc.schedule, "lerd-php84-fpm", false,
			); err != nil {
				t.Fatalf("writeWorkerUnitFile: %v", err)
			}
			data, err := os.ReadFile(unitPath(tc.unitName))
			if err != nil {
				t.Fatalf("read unit: %v", err)
			}
			unit := string(data)
			if !strings.Contains(unit, "--env=FORCE_COLOR=1") {
				t.Errorf("unit should pass the colour env to podman exec:\n%s", unit)
			}
			if !strings.Contains(unit, "lerd-php84-fpm php artisan queue:work") {
				t.Errorf("colour flags should sit before the container name:\n%s", unit)
			}
		})
	}

	if _, err := writeWorkerUnitFile(
		"lerd-vite-alpha", "Vite", "alpha", sitePath, "",
		"npm run dev", "always", "", "", true,
	); err != nil {
		t.Fatalf("writeWorkerUnitFile host: %v", err)
	}
	host, err := os.ReadFile(unitPath("lerd-vite-alpha"))
	if err != nil {
		t.Fatalf("read host unit: %v", err)
	}
	if !strings.Contains(string(host), `Environment="FORCE_COLOR=1"`) {
		t.Errorf("host worker unit should set the colour env:\n%s", host)
	}
}

func TestWriteWorkerUnitFileRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if _, err := writeWorkerUnitFile(
		"lerd-queue-alpha", "Queue Worker", "alpha", t.TempDir(), "8.4",
		"php artisan queue:work", "always", "", "lerd-php84-fpm", false,
	); err != nil {
		t.Fatalf("writeWorkerUnitFile: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "systemd", "user", "lerd-queue-alpha.service"))
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	unit := string(data)
	if strings.Contains(unit, "FORCE_COLOR") {
		t.Errorf("NO_COLOR should leave the unit unchanged:\n%s", unit)
	}
	if !strings.Contains(unit, "--env=LERD_SITE=alpha lerd-php84-fpm php artisan queue:work") {
		t.Errorf("empty colour args should not leave a double space:\n%s", unit)
	}
}
