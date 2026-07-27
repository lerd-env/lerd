//go:build linux

package cli

import (
	"errors"
	"strings"
	"testing"
)

// stubSystemSetup drives the three preflight conditions and captures both the
// sudo re-exec and the fallback, so the root pass can be exercised without a
// real kernel, login manager, or sudo.
func stubSystemSetup(t *testing.T, portsNeeded, lingerOff, sudoersReady bool) (*[][]string, *[]bool) {
	t.Helper()
	origPorts, origLinger, origSudoers := unprivPortsNeeded, lingerNeeded, dnsSudoersReady
	origRunner, origLegacy, origRecord := sudoSelfRunner, legacySystemSetup, recordDNSSudoers
	t.Cleanup(func() {
		unprivPortsNeeded, lingerNeeded, dnsSudoersReady = origPorts, origLinger, origSudoers
		sudoSelfRunner, legacySystemSetup, recordDNSSudoers = origRunner, origLegacy, origRecord
	})
	unprivPortsNeeded = func() bool { return portsNeeded }
	lingerNeeded = func() bool { return lingerOff }
	dnsSudoersReady = func() bool { return sudoersReady }
	recordDNSSudoers = func(string) {}

	var runs [][]string
	sudoSelfRunner = func(args ...string) error {
		runs = append(runs, args)
		return nil
	}
	var legacy []bool
	legacySystemSetup = func(wantDNS bool) error {
		legacy = append(legacy, wantDNS)
		return nil
	}
	return &runs, &legacy
}

// The common case is a reinstall where every root step already applies. It must
// stay completely silent rather than asking for a password to do nothing.
func TestRunSystemSetupSkipsWhenAlreadyApplied(t *testing.T) {
	runs, legacy := stubSystemSetup(t, false, false, true)

	if err := runSystemSetup(true); err != nil {
		t.Fatalf("runSystemSetup: %v", err)
	}
	if len(*runs) != 0 || len(*legacy) != 0 {
		t.Errorf("escalated with nothing to do: sudo %v, fallback %v", *runs, *legacy)
	}
}

// A localhost-mode install never configures a resolver, so a missing DNS grant
// is not a reason to ask for root.
func TestRunSystemSetupIgnoresSudoersWhenDNSUnmanaged(t *testing.T) {
	runs, _ := stubSystemSetup(t, false, false, false)

	if err := runSystemSetup(false); err != nil {
		t.Fatalf("runSystemSetup: %v", err)
	}
	if len(*runs) != 0 {
		t.Errorf("escalated for a DNS grant a localhost install never uses: %v", *runs)
	}
}

func TestRunSystemSetupOneSudoCall(t *testing.T) {
	runs, legacy := stubSystemSetup(t, true, true, false)

	if err := runSystemSetup(true); err != nil {
		t.Fatalf("runSystemSetup: %v", err)
	}
	if len(*runs) != 1 {
		t.Fatalf("sudo calls = %d, want exactly 1 (%v)", len(*runs), *runs)
	}
	got := strings.Join((*runs)[0], " ")
	if got != "bootstrap --system" {
		t.Errorf("sudo args = %q, want %q", got, "bootstrap --system")
	}
	if len(*legacy) != 0 {
		t.Errorf("fallback ran despite a successful sudo pass: %v", *legacy)
	}
}

func TestRunSystemSetupSkipsSudoersForLocalhostMode(t *testing.T) {
	runs, _ := stubSystemSetup(t, true, false, false)

	if err := runSystemSetup(false); err != nil {
		t.Fatalf("runSystemSetup: %v", err)
	}
	if len(*runs) != 1 {
		t.Fatalf("sudo calls = %d, want 1 (%v)", len(*runs), *runs)
	}
	got := strings.Join((*runs)[0], " ")
	if got != "bootstrap --system --skip-sudoers" {
		t.Errorf("sudo args = %q, want the sudoers grant left out", got)
	}
}

// sudo hands the root pass its own HOME, so the marker it writes is invisible to
// the user. Without recording it on this side, every later install would decide
// the grant is missing and escalate again.
func TestRunSystemSetupRecordsSudoersMarker(t *testing.T) {
	stubSystemSetup(t, true, false, false)
	var recorded []string
	recordDNSSudoers = func(user string) { recorded = append(recorded, user) }

	if err := runSystemSetup(true); err != nil {
		t.Fatalf("runSystemSetup: %v", err)
	}
	if len(recorded) != 1 || recorded[0] != currentUserName() {
		t.Errorf("marker recorded for %v, want [%s]", recorded, currentUserName())
	}

	// A localhost-mode install writes no grant, so there is nothing to record.
	recorded = nil
	if err := runSystemSetup(false); err != nil {
		t.Fatalf("runSystemSetup: %v", err)
	}
	if len(recorded) != 0 {
		t.Errorf("marker recorded with no grant written: %v", recorded)
	}
}

// No sudo, or a user who cannot use it, must land on the old per-step path
// rather than failing the install outright.
func TestRunSystemSetupFallsBackWhenSudoFails(t *testing.T) {
	_, legacy := stubSystemSetup(t, true, false, false)
	sudoSelfRunner = func(...string) error { return errors.New("sudo: command not found") }

	if err := runSystemSetup(true); err != nil {
		t.Fatalf("runSystemSetup: %v", err)
	}
	if len(*legacy) != 1 || !(*legacy)[0] {
		t.Errorf("fallback runs = %v, want one call with DNS managed", *legacy)
	}
}
