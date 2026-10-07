//go:build linux

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The daemons are the loudest symptom: their ExecStart names the retired keg,
// so systemd can only report exit status 203 until the unit is rewritten.
func TestHealDaemonUnitsRewritesUnitsWithAGoneBinary(t *testing.T) {
	writeInstalledUnit(t, "lerd-ui", "/home/linuxbrew/.linuxbrew/Cellar/lerd/1.31.0/bin/lerd serve-ui")
	fake := &fakeServiceMgr{writeChanged: true}
	swapMgr(t, fake)

	healed := healDaemonUnits(binaryGone)

	if !equalStrings(fake.calls, []string{"write:lerd-ui", "reload"}) {
		t.Errorf("expected the lerd-ui unit to be rewritten and reloaded, got %v", fake.calls)
	}
	if !equalStrings(healed, []string{"lerd-ui"}) {
		t.Errorf("healDaemonUnits() = %v; want the repair reported so the start can say so", healed)
	}
}

// A daemon whose binary is where the unit says it is must be left alone: a
// start from a checkout build has no business repointing the login daemons.
func TestHealDaemonUnitsLeavesResolvableUnitsAlone(t *testing.T) {
	live := filepath.Join(t.TempDir(), "lerd")
	if err := os.WriteFile(live, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	writeInstalledUnit(t, "lerd-ui", live+" serve-ui")
	fake := &fakeServiceMgr{writeChanged: true}
	swapMgr(t, fake)

	if healed := healDaemonUnits(binaryGone); len(healed) != 0 {
		t.Errorf("healDaemonUnits() = %v; want nothing repaired", healed)
	}

	if len(fake.calls) != 0 {
		t.Errorf("a working unit was rewritten: %v", fake.calls)
	}
}

// A host with no tray helper installed carries a lerd-tray unit whose
// ExecStart is missing and stays missing however often it is rewritten.
// Repairing that on every start would report work it never did.
func TestHealDaemonUnitsSkipsAUnitTheRewriteCannotFix(t *testing.T) {
	writeInstalledUnit(t, "lerd-tray", filepath.Join(filepath.Dir(config.LerdBinary()), "lerd-tray"))
	fake := &fakeServiceMgr{writeChanged: true}
	swapMgr(t, fake)

	if healed := healDaemonUnits(binaryGone); len(healed) != 0 {
		t.Errorf("healDaemonUnits() = %v; want nothing reported", healed)
	}
	if len(fake.calls) != 0 {
		t.Errorf("the unit was rewritten to the same content: %v", fake.calls)
	}
}

// writeInstalledUnit puts a unit file with the given ExecStart in a temp
// systemd user dir, standing in for what a previous install left behind.
func writeInstalledUnit(t *testing.T, name, execStart string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := config.SystemdUserDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	unit := "[Service]\nType=notify\nExecStart=" + execStart + "\n"
	if err := os.WriteFile(filepath.Join(dir, name+".service"), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}
}

// lerd-dns runs the lerd binary too, and when it cannot start the resolver
// has nothing to send .test to, or every query on a host routing ~. through
// it. Its unit is built in code, so the heal has to know how to rebuild it.
func TestHealDaemonUnitsRewritesLerdDNS(t *testing.T) {
	writeInstalledUnit(t, "lerd-dns", "/home/linuxbrew/.linuxbrew/Cellar/lerd/1.31.0/bin/lerd dns-serve --listen 127.0.0.1")
	fake := &fakeServiceMgr{writeChanged: true}
	swapMgr(t, fake)

	healed := healDaemonUnits(binaryGone)

	if !equalStrings(healed, []string{"lerd-dns"}) {
		t.Errorf("healDaemonUnits() = %v; want lerd-dns repointed", healed)
	}
}
