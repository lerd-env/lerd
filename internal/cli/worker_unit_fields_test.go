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
			// half-written for systemd to pick up.
			for _, ext := range []string{".service", ".timer"} {
				path := filepath.Join(tmp, "systemd", "user", "lerd-probe-mysite"+ext)
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					continue
				}
				if strings.Contains(string(b), "ExecStartPre=") {
					t.Errorf("%s: a unit carrying the injected directive was written to %s", tc.field, path)
				}
			}
		})
	}
}

// The ordinary shape still writes.
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
	b, err := os.ReadFile(filepath.Join(tmp, "systemd", "user", "lerd-queue-mysite.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Description=Lerd Queue Worker (mysite)") {
		t.Errorf("unit lost its description:\n%s", b)
	}
}
