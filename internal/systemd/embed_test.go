package systemd

import (
	"strings"
	"testing"
)

func TestGetUnitResolvesBinaryPath(t *testing.T) {
	orig := lerdBinaryPath
	t.Cleanup(func() { lerdBinaryPath = orig })
	lerdBinaryPath = func() string { return "/usr/bin/lerd" }

	ui, err := GetUnit("lerd-ui")
	if err != nil {
		t.Fatalf("GetUnit: %v", err)
	}
	if !strings.Contains(ui, "ExecStart=/usr/bin/lerd serve-ui") {
		t.Errorf("lerd-ui ExecStart not resolved:\n%s", ui)
	}
	if strings.Contains(ui, "%h/.local/bin/lerd") {
		t.Errorf("lerd-ui still has the template path:\n%s", ui)
	}

	tray, err := GetUnit("lerd-tray")
	if err != nil {
		t.Fatalf("GetUnit tray: %v", err)
	}
	if !strings.Contains(tray, "ExecStart=/usr/bin/lerd-tray") {
		t.Errorf("lerd-tray ExecStart not resolved:\n%s", tray)
	}
}

// When the binary path can't be resolved, the template default is left intact
// rather than producing a broken ExecStart.
func TestGetUnitKeepsTemplateWhenUnresolved(t *testing.T) {
	orig := lerdBinaryPath
	t.Cleanup(func() { lerdBinaryPath = orig })
	lerdBinaryPath = func() string { return "" }

	ui, err := GetUnit("lerd-ui")
	if err != nil {
		t.Fatalf("GetUnit: %v", err)
	}
	if !strings.Contains(ui, "%h/.local/bin/lerd serve-ui") {
		t.Errorf("expected template default, got:\n%s", ui)
	}
}

// Windows holds 7073 for two minutes when WSL restarts under an open dashboard,
// past systemd's default five quick tries, and giving up leaves no dashboard.
func TestLerdUIUnitKeepsRetryingABusyPort(t *testing.T) {
	ui, err := GetUnit("lerd-ui")
	if err != nil {
		t.Fatalf("GetUnit: %v", err)
	}
	unit, service, ok := strings.Cut(ui, "[Service]")
	if !ok {
		t.Fatalf("no [Service] section:\n%s", ui)
	}
	if !strings.Contains(unit, "StartLimitIntervalSec=0\n") {
		t.Errorf("[Unit] must lift the start limit:\n%s", ui)
	}
	if !strings.Contains(service, "RestartSec=2\n") {
		t.Errorf("[Service] must space the retries out:\n%s", ui)
	}
}
