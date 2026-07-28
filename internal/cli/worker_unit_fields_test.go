package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// injection is a payload that opens its own section and adds a directive
// systemd will execute. Every value spliced into a generated unit is a line of
// that unit, so any of them carrying a newline can do this.
const injection = "\n[Service]\nExecStartPre=/usr/bin/touch /tmp/lerd-unit-injection-proof\nDescription=x"

// Only the command used to be checked, and the check ran before the unit was
// assembled, so every other value a project controls reached the file
// unexamined. A .lerd.yaml custom_workers entry could put an ExecStartPre into
// a unit through its label, its restart or its schedule.
func TestWriteWorkerUnitFileRefusesInjectionInAnyField(t *testing.T) {
	cases := []struct {
		field                             string
		label, restart, schedule, fpmUnit string
		siteName                          string
	}{
		{field: "label", label: "Vite" + injection, restart: "on-failure", siteName: "mysite"},
		{field: "restart", label: "Vite", restart: "on-failure" + injection, siteName: "mysite"},
		{field: "schedule", label: "Vite", restart: "on-failure", schedule: "*:0/5" + injection, siteName: "mysite"},
		{field: "siteName", label: "Vite", restart: "on-failure", siteName: "mysite" + injection},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", tmp)
			t.Setenv("XDG_DATA_HOME", tmp)

			_, err := writeWorkerUnitFile(
				"lerd-probe-mysite", tc.label, tc.siteName, t.TempDir(), "8.4",
				"sleep 3600", tc.restart, tc.schedule, "lerd-php84-fpm", false,
			)
			if err == nil {
				t.Errorf("%s carrying a newline was accepted", tc.field)
			}
			// Nothing may reach disk either, so a refused unit cannot be left
			// half-written for the service manager to pick up. The whole tree is
			// walked rather than named files, because the artifact differs by
			// platform and a name-based check passes vacuously on the other one.
			if found := fileContaining(t, tmp, "ExecStartPre="); found != "" {
				t.Errorf("%s: a unit carrying the injected directive was written to %s", tc.field, found)
			}
		})
	}
}

// The ordinary shape still writes. What the unit says is asserted per platform,
// since Linux writes a systemd unit and macOS a launchd plist.
func TestWriteWorkerUnitFileAcceptsNormalFields(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	changed, err := writeWorkerUnitFile(
		"lerd-queue-mysite", "Queue Worker", "mysite", t.TempDir(), "8.4",
		"php artisan queue:work", "always", "", "lerd-php84-fpm", false,
	)
	if err != nil || !changed {
		t.Fatalf("writeWorkerUnitFile = %v, %v; want a clean write", changed, err)
	}
}

// fileContaining returns the first file under root holding needle, or empty.
func fileContaining(t *testing.T, root, needle string) string {
	t.Helper()
	var found string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || found != "" {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), needle) {
			found = p
		}
		return nil
	})
	return found
}
