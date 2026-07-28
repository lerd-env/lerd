//go:build linux

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The unit a clean write produces, asserted where systemd is the target.
func TestWriteWorkerUnitFileWritesTheDescription(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if _, err := writeWorkerUnitFile(
		"lerd-queue-mysite", "Queue Worker", "mysite", t.TempDir(), "8.4",
		"php artisan queue:work", "always", "", "lerd-php84-fpm", false,
	); err != nil {
		t.Fatalf("writeWorkerUnitFile: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(tmp, "systemd", "user", "lerd-queue-mysite.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Description=Lerd Queue Worker (mysite)") {
		t.Errorf("unit lost its description:\n%s", b)
	}
}
