package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func reportWith(findings ...Finding) DoctorReport {
	return DoctorReport{Findings: findings}
}

func TestRunDoctorFixNothingToDo(t *testing.T) {
	var buf bytes.Buffer
	rep := reportWith(Finding{Name: "port 80", Status: "fail"}) // no fix attached
	if err := runDoctorFix(&buf, rep, false, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Nothing to fix automatically") {
		t.Fatalf("expected nothing-to-fix message, got: %q", buf.String())
	}
}

func TestRunDoctorFixDryRunChangesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	var buf bytes.Buffer
	rep := reportWith(
		Finding{Name: "data dir", Status: "fail", Fix: autoFix(fixMkdir, dir, "create the data directory")},
		Finding{Name: "crun", Status: "fail", Hint: "sudo apt install crun", Fix: manualFix},
		// warn-level manual finding keeps its guidance in Message, not Hint.
		Finding{Name: "certutil", Status: "warn", Message: "run rpm-ostree install nss-tools", Fix: manualFix},
	)
	if err := runDoctorFix(&buf, rep, false, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Dry run") {
		t.Fatalf("expected dry-run notice, got: %q", out)
	}
	if !strings.Contains(out, "create the data directory") || !strings.Contains(out, "sudo apt install crun") {
		t.Fatalf("dry run should list both auto and manual fixes, got: %q", out)
	}
	if !strings.Contains(out, "run rpm-ostree install nss-tools") {
		t.Fatalf("warn-level manual finding should show its Message guidance, got: %q", out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("dry run must not create the directory")
	}
}

// TestRunDoctorFixLeavesPrivilegedRepairsToTheUser is the end-to-end shape of
// the fix: under --yes, which is exactly how the MCP diag tool invokes it, a
// repair needing sudo is listed for the user and never applied.
func TestRunDoctorFixLeavesPrivilegedRepairsToTheUser(t *testing.T) {
	orig := reCheckReport
	reCheckReport = func() (DoctorReport, error) { return DoctorReport{}, nil }
	defer func() { reCheckReport = orig }()

	var buf bytes.Buffer
	rep := reportWith(
		Finding{Name: "resolver hookup", Status: "fail",
			Fix: manualFixWith("run `lerd dns:repair` (it needs sudo to rewrite the resolver config)")},
		Finding{Name: "podman events_logger journald", Status: "warn",
			Fix: manualFixWith("run `lerd wsl:setup` (it needs sudo to write the podman config)")},
	)
	if err := runDoctorFix(&buf, rep, true, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "These need elevated privileges") {
		t.Errorf("privileged repairs were not listed for the user: %q", out)
	}
	if !strings.Contains(out, "lerd dns:repair") || !strings.Contains(out, "lerd wsl:setup") {
		t.Errorf("guidance should name the exact command: %q", out)
	}
	if strings.Contains(out, "Applied 1 fix") || strings.Contains(out, "Applied 2 fix") {
		t.Errorf("nothing should have been applied: %q", out)
	}
	if !strings.Contains(out, "Nothing to fix automatically") && !strings.Contains(out, "Applied 0 fix") {
		t.Errorf("expected an empty auto tier: %q", out)
	}
}

func TestRunDoctorFixAppliesAndReChecks(t *testing.T) {
	// Auto-apply (yes=true) a non-heavy mkdir fix, stub the re-check clean.
	orig := reCheckReport
	reCheckReport = func() (DoctorReport, error) { return DoctorReport{}, nil }
	defer func() { reCheckReport = orig }()

	dir := filepath.Join(t.TempDir(), "quadlets")
	var buf bytes.Buffer
	rep := reportWith(
		Finding{Name: "service config dir", Status: "fail", Fix: autoFix(fixMkdir, dir, "create the service config directory")},
		Finding{Name: "netavark", Status: "fail", Hint: "sudo apt install netavark", Fix: manualFix},
	)
	if err := runDoctorFix(&buf, rep, true, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("mkdir fix did not run: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Applied 1 fix") {
		t.Fatalf("expected applied summary, got: %q", out)
	}
	if !strings.Contains(out, "no automatic fixes remain") {
		t.Fatalf("expected clean re-check, got: %q", out)
	}
	if !strings.Contains(out, "sudo apt install netavark") {
		t.Fatalf("manual fixes should still be listed, got: %q", out)
	}
}
