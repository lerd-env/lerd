package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyDoctorFixRejectsNonAuto(t *testing.T) {
	var buf bytes.Buffer
	if err := ApplyDoctorFix(nil, &buf); err == nil {
		t.Fatal("nil fix should error")
	}
	if err := ApplyDoctorFix(manualFix, &buf); err == nil {
		t.Fatal("manual fix should not be applied")
	}
}

func TestApplyDoctorFixUnknownKey(t *testing.T) {
	var buf bytes.Buffer
	if err := ApplyDoctorFix(&DoctorFix{Tier: FixAuto, Key: "bogus"}, &buf); err == nil {
		t.Fatal("unknown key should error")
	}
}

func TestApplyDoctorFixMkdirCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "quadlets")
	var buf bytes.Buffer
	if err := ApplyDoctorFix(autoFix(fixMkdir, dir, "create it"), &buf); err != nil {
		t.Fatalf("mkdir fix failed: %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
	if !strings.Contains(buf.String(), dir) {
		t.Fatalf("output did not mention the target: %q", buf.String())
	}
}

func TestApplyDoctorFixMkdirEmptyArg(t *testing.T) {
	var buf bytes.Buffer
	if err := ApplyDoctorFix(autoFix(fixMkdir, "", "create it"), &buf); err == nil {
		t.Fatal("empty mkdir target should error")
	}
}

// TestApplyDoctorFixRejectsPrivilegedKeys pins the auto-tier contract: a fix
// that shells into a command running sudo must not be dispatchable, so
// re-adding one to the auto tier fails here rather than silently letting
// `doctor --fix --yes` and the MCP diag tool elevate unattended.
func TestApplyDoctorFixRejectsPrivilegedKeys(t *testing.T) {
	for _, key := range []string{fixDNSRepair, fixWSLSetup} {
		var buf bytes.Buffer
		if err := ApplyDoctorFix(&DoctorFix{Tier: FixAuto, Key: key}, &buf); err == nil {
			t.Errorf("%q is dispatchable from the auto tier but it runs sudo", key)
		}
	}
}

func TestHeavyFixClassification(t *testing.T) {
	if !IsHeavyFix(autoFix(fixCleanup, "", "")) || !IsHeavyFix(autoFix(fixInstall, "", "")) {
		t.Fatal("cleanup and install should be heavy")
	}
	if IsHeavyFix(autoFix(fixMkdir, "/x", "")) {
		t.Fatal("mkdir should not be heavy")
	}
}

// The network-online repair used to re-enter `lerd start`, which reconfigures
// the resolver through sudo, elevating unattended under `doctor --fix --yes`.
// It now installs a user-level drop-in in-process; assert it routes there and
// never touches privilege, and that "start" is no longer a dispatchable key.
func TestApplyDoctorFixNetworkWaitStaysUnprivileged(t *testing.T) {
	called := false
	orig := ensureNoNetworkWaitStallFn
	ensureNoNetworkWaitStallFn = func() (bool, error) { called = true; return false, nil }
	t.Cleanup(func() { ensureNoNetworkWaitStallFn = orig })

	fix := autoFix(fixNetworkWait, "", "install the podman network-online drop-in")
	var buf bytes.Buffer
	if err := ApplyDoctorFix(fix, &buf); err != nil {
		t.Fatalf("network-wait fix failed: %v", err)
	}
	if !called {
		t.Error("network-wait fix did not route to the in-process drop-in installer")
	}
	if IsHeavyFix(fix) {
		t.Error("network-wait fix should not be heavy")
	}

	// The privileged `start` fix is gone: a doctor that re-adds it lands in the
	// unknown-key default rather than silently running sudo from the auto tier.
	if err := ApplyDoctorFix(&DoctorFix{Tier: FixAuto, Key: "start"}, &buf); err == nil {
		t.Error(`"start" is dispatchable from the auto tier again but it runs lerd start (sudo)`)
	}
}
