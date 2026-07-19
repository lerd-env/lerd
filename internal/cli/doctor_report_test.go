package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDoctorReportJSONShapeAndTiers(t *testing.T) {
	rep := DoctorReport{Version: "1.2.3"}
	rep.add(Finding{Section: "Prerequisites", Name: "data dir", Status: "fail", Message: "not writable", Hint: "mkdir -p /x"})
	rep.fixLast(autoFix(fixMkdir, "/x", "create the data directory"))
	rep.add(Finding{Section: "Prerequisites", Name: "crun", Status: "warn", Hint: "sudo apt install crun"})
	rep.fixLast(manualFix)
	rep.Failures, rep.Warnings = 1, 1

	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)

	// Tiers must serialize as stable strings the MCP tool branches on.
	if !strings.Contains(s, `"tier":"auto"`) || !strings.Contains(s, `"tier":"manual"`) {
		t.Fatalf("expected string tiers in JSON: %s", s)
	}
	// The internal Arg must not leak into the machine contract.
	if strings.Contains(s, "/x\",\"") && strings.Contains(s, `"arg"`) {
		t.Fatalf("arg should be omitted from JSON: %s", s)
	}
	if !strings.Contains(s, `"version":"1.2.3"`) {
		t.Fatalf("expected version in JSON: %s", s)
	}

	// Round-trips into the shape MCP unmarshals.
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back["failures"].(float64) != 1 || back["warnings"].(float64) != 1 {
		t.Fatalf("counts wrong: %v", back)
	}
}

func TestDoctorReportFixLastAttachesToLatest(t *testing.T) {
	rep := &DoctorReport{}
	rep.add(Finding{Name: "first", Status: "ok"})
	rep.add(Finding{Name: "second", Status: "fail"})
	rep.fixLast(autoFix("mkdir", "/data", "create it"))

	if rep.Findings[0].Fix != nil {
		t.Fatalf("fixLast attached to the wrong finding: %q got a fix", rep.Findings[0].Name)
	}
	got := rep.Findings[1].Fix
	if got == nil || got.Tier != FixAuto || got.Key != "mkdir" || got.Arg != "/data" {
		t.Fatalf("fixLast did not tag the latest finding: %+v", got)
	}
}

func TestDoctorReportFixLastNoFindingsIsNoop(t *testing.T) {
	rep := &DoctorReport{}
	rep.fixLast(manualFix) // must not panic on an empty report
	if len(rep.Findings) != 0 {
		t.Fatalf("fixLast on empty report added a finding")
	}
}

func TestDoctorReportPartitionsFixesByTier(t *testing.T) {
	rep := &DoctorReport{}
	rep.add(Finding{Name: "ok-check", Status: "ok"})
	rep.add(Finding{Name: "auto-check", Status: "fail"})
	rep.fixLast(autoFix("install", "", "install services"))
	rep.add(Finding{Name: "manual-check", Status: "fail"})
	rep.fixLast(manualFix)
	rep.add(Finding{Name: "external-check", Status: "fail"}) // no fix attached

	auto := rep.AutoFixes()
	if len(auto) != 1 || auto[0].Name != "auto-check" {
		t.Fatalf("AutoFixes returned %+v, want only auto-check", auto)
	}
	manual := rep.ManualFixes()
	if len(manual) != 1 || manual[0].Name != "manual-check" {
		t.Fatalf("ManualFixes returned %+v, want only manual-check", manual)
	}
}

func TestManualFixTier(t *testing.T) {
	if manualFix.Tier != FixManual {
		t.Fatalf("manualFix tier = %d, want FixManual", manualFix.Tier)
	}
}
