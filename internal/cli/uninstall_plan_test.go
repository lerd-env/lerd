package cli

import (
	"strings"
	"testing"
)

// Without a terminal nobody can confirm, and a script must not read the
// refusal as a finished uninstall.
func TestUninstallPlan_AbortIsAnError(t *testing.T) {
	_, err := uninstallPlanFor(false, false)
	if err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("err = %v, want an aborted error", err)
	}
}

// --keep-data is the unattended uninstall that keeps what a reinstall needs:
// no prompt, the data and images stay, everything else goes.
func TestUninstallPlan_KeepDataAsksNothingAndKeepsData(t *testing.T) {
	p, err := uninstallPlanFor(false, true)
	if err != nil {
		t.Fatalf("keep-data asked or aborted: %v", err)
	}
	if p.removeData || p.purgeImages {
		t.Errorf("plan = %+v, want the data and images kept", p)
	}
	if !p.removeMCP || !p.removeMkcertCA {
		t.Errorf("plan = %+v, want the rest removed as --force does", p)
	}
}

func TestUninstallPlan_KeepDataWinsOverForceForData(t *testing.T) {
	p, err := uninstallPlanFor(true, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.removeData {
		t.Error("--force --keep-data removed the data")
	}
}

func TestUninstallPlan_ForceRemovesEverything(t *testing.T) {
	p, err := uninstallPlanFor(true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !p.removeData || !p.removeMCP || !p.removeMkcertCA || !p.purgeImages {
		t.Errorf("plan = %+v, want everything removed", p)
	}
}
