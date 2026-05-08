//go:build linux

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHostWorkerUnitFile_useFnmExec(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	binDir := filepath.Join(tmp, "lerd", "bin")
	os.MkdirAll(binDir, 0755)
	os.WriteFile(filepath.Join(binDir, "fnm"), []byte("#!/bin/sh"), 0755)

	sitePath := t.TempDir()
	os.WriteFile(filepath.Join(sitePath, ".node-version"), []byte("20"), 0644)

	changed, err := writeWorkerUnitFile(
		"lerd-vite-mysite", "Vite Dev Server", "mysite",
		sitePath, "8.4", "npm run dev",
		"on-failure", "", "lerd-php84-fpm", true,
	)
	if err != nil {
		t.Fatalf("writeWorkerUnitFile (host): %v", err)
	}
	if !changed {
		t.Error("first write reported changed=false, want true")
	}

	unitPath := filepath.Join(tmp, "systemd", "user", "lerd-vite-mysite.service")
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	unit := string(data)

	if strings.Contains(unit, "podman") {
		t.Error("host worker must not use podman exec")
	}
	if !strings.Contains(unit, "fnm") {
		t.Error("host worker must use fnm exec")
	}
	if !strings.Contains(unit, "--using=20") {
		t.Errorf("expected --using=20 from .node-version, got:\n%s", unit)
	}
	if !strings.Contains(unit, "WorkingDirectory=") {
		t.Error("host worker must set WorkingDirectory")
	}
	if !strings.Contains(unit, "npm run dev") {
		t.Error("host worker must include the command")
	}
	if !strings.Contains(unit, "Restart=on-failure") {
		t.Error("host worker must respect restart policy")
	}
	if strings.Contains(unit, "BindsTo=") {
		t.Error("host worker must not bind to FPM container")
	}
}

func TestWriteWorkerUnitFile_hostFalse_usesPodman(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	changed, err := writeWorkerUnitFile(
		"lerd-horizon-mysite", "Horizon", "mysite",
		"/srv/mysite", "8.4", "php artisan horizon",
		"always", "", "lerd-php84-fpm", false,
	)
	if err != nil {
		t.Fatalf("writeWorkerUnitFile (container): %v", err)
	}
	if !changed {
		t.Error("first write reported changed=false")
	}

	unitPath := filepath.Join(tmp, "systemd", "user", "lerd-horizon-mysite.service")
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	unit := string(data)

	if !strings.Contains(unit, "podman") {
		t.Error("container worker must use podman exec")
	}
	if !strings.Contains(unit, "BindsTo=lerd-php84-fpm.service") {
		t.Error("container worker must bind to FPM unit")
	}
}
