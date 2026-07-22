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

// TestAutoFixesDedupesByKey covers the repeated-repair case: dns.Diagnose can
// fail several steps that each attach the same repair, and without deduping the
// whole sequence would run once per failing step.
func TestAutoFixesDedupesByKey(t *testing.T) {
	rep := &DoctorReport{}
	for _, name := range []string{"resolver hookup", "interface routing", "system lookup"} {
		rep.add(Finding{Name: name, Status: "fail"})
		rep.fixLast(autoFix(fixNetworkWait, "", "install the drop-in"))
	}
	rep.add(Finding{Name: "data dir", Status: "fail"})
	rep.fixLast(autoFix(fixMkdir, "/x", "create it"))

	autos := rep.AutoFixes()
	if len(autos) != 2 {
		t.Fatalf("got %d auto fixes, want 2 (one per distinct key): %+v", len(autos), autos)
	}
	if autos[0].Fix.Key != fixNetworkWait || autos[1].Fix.Key != fixMkdir {
		t.Errorf("report order not preserved: %q then %q", autos[0].Fix.Key, autos[1].Fix.Key)
	}
	if autos[0].Name != "resolver hookup" {
		t.Errorf("first occurrence should win, got %q", autos[0].Name)
	}
}

// mkdir and php-rebuild reuse one key across findings that each target a
// different directory or version; those are distinct fixes and every one must
// survive the dedupe, or --fix silently repairs only the first.
func TestAutoFixesKeepsSameKeyWithDistinctArgs(t *testing.T) {
	rep := &DoctorReport{}
	for _, dir := range []string{"/a", "/b", "/c"} {
		rep.add(Finding{Name: "parked dir: " + dir, Status: "warn"})
		rep.fixLast(autoFix(fixMkdir, dir, "create the parked directory"))
	}
	for _, v := range []string{"8.3", "8.4"} {
		rep.add(Finding{Name: "PHP " + v + " image", Status: "fail"})
		rep.fixLast(autoFix(fixPhpRebuild, v, "rebuild the image"))
	}

	autos := rep.AutoFixes()
	if len(autos) != 5 {
		t.Fatalf("got %d auto fixes, want 5 (one per distinct arg): %+v", len(autos), autos)
	}
	args := map[string]bool{}
	for _, f := range autos {
		args[f.Fix.Key+" "+f.Fix.Arg] = true
	}
	for _, want := range []string{"mkdir /a", "mkdir /b", "mkdir /c", "php-rebuild 8.3", "php-rebuild 8.4"} {
		if !args[want] {
			t.Errorf("missing fix %q from %v", want, args)
		}
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

func TestDoctorReportSplitsRequiredFromOptionalAutoFixes(t *testing.T) {
	rep := &DoctorReport{}
	rep.add(Finding{Name: "data dir", Status: "fail"})
	rep.fixLast(autoFix(fixMkdir, "/x", "create the data directory"))
	rep.add(Finding{Name: "Reclaimable disk", Status: "info"})
	rep.fixLast(autoFix(fixCleanup, "", "reclaim disk space (lerd cleanup)"))

	req := rep.RequiredAutoFixes()
	if len(req) != 1 || req[0].Name != "data dir" {
		t.Fatalf("RequiredAutoFixes returned %+v, want only data dir", req)
	}
	opt := rep.OptionalAutoFixes()
	if len(opt) != 1 || opt[0].Name != "Reclaimable disk" {
		t.Fatalf("OptionalAutoFixes returned %+v, want only Reclaimable disk", opt)
	}
}

func TestManualFixTier(t *testing.T) {
	if manualFix.Tier != FixManual {
		t.Fatalf("manualFix tier = %d, want FixManual", manualFix.Tier)
	}
}
