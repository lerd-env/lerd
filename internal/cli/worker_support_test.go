package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// setAutostart writes a global config with the autostart flag in the given
// state so a test can exercise both sides of the boot-arming gate.
func setAutostart(t *testing.T, enabled bool) {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	dir := filepath.Join(cfgHome, "lerd")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "autostart:\n  disabled: " + strconv.FormatBool(!enabled) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// setEnableArmsBootOnly substitutes the platform fact so both behaviours are
// covered from either build.
func setEnableArmsBootOnly(t *testing.T, v bool) {
	t.Helper()
	prev := enableArmsBootOnly
	enableArmsBootOnly = v
	t.Cleanup(func() { enableArmsBootOnly = prev })
}

// TestSyncWorkerBootArming_autostartDisabled pins issue #1531: while autostart is
// off, arming a worker for login start is skipped. Enabling would drop the
// default.target.wants symlink back in and the next boot would start the worker
// with no lerd services behind it, restarting it forever.
func TestSyncWorkerBootArming_autostartDisabled(t *testing.T) {
	setEnableArmsBootOnly(t, true)
	setAutostart(t, false)
	mgr := &captureEnableMgr{}
	swapMgr(t, mgr)

	if err := syncWorkerBootArming("lerd-horizon-ws"); err != nil {
		t.Fatalf("syncWorkerBootArming: %v", err)
	}
	if len(mgr.enabled) != 0 {
		t.Errorf("enabled %v, want nothing enabled while autostart is off", mgr.enabled)
	}
	if len(mgr.disableCalls) != 1 || mgr.disableCalls[0] != "lerd-horizon-ws" {
		t.Errorf("disabled %v, want [lerd-horizon-ws]: an arming left by an older build has to be cleared", mgr.disableCalls)
	}
}

// With autostart on, the same call arms the unit as it always did.
func TestSyncWorkerBootArming_autostartEnabled(t *testing.T) {
	setEnableArmsBootOnly(t, true)
	setAutostart(t, true)
	mgr := &captureEnableMgr{}
	swapMgr(t, mgr)

	if err := syncWorkerBootArming("lerd-horizon-ws"); err != nil {
		t.Fatalf("syncWorkerBootArming: %v", err)
	}
	if len(mgr.enabled) != 1 || mgr.enabled[0] != "lerd-horizon-ws" {
		t.Errorf("enabled %v, want [lerd-horizon-ws]", mgr.enabled)
	}
}

// On a platform where enable is also what makes a unit startable at all, the
// autostart flag must not gate it: skipping would stop the worker running now.
func TestSyncWorkerBootArming_enableAlsoStarts_ignoresAutostart(t *testing.T) {
	setEnableArmsBootOnly(t, false)
	setAutostart(t, false)
	mgr := &captureEnableMgr{}
	swapMgr(t, mgr)

	if err := syncWorkerBootArming("lerd-horizon-ws"); err != nil {
		t.Fatalf("syncWorkerBootArming: %v", err)
	}
	if len(mgr.enabled) != 1 {
		t.Errorf("enabled %v, want the unit enabled regardless of autostart", mgr.enabled)
	}
}

// captureEnableMgr records Enable calls without touching the real init system.
type captureEnableMgr struct {
	stopTrackingMgr
	enabled []string
}

func (c *captureEnableMgr) Enable(name string) error {
	c.enabled = append(c.enabled, name)
	return nil
}

// TestWorkerStartForSite_skipsLifecycleWhenUnsupported pins the contract
// fixed in this round: when workerSupportedOnPlatform reports the worker
// can't run on this platform, WorkerStartForSite must return nil without
// calling Enable/StartUnit. On macOS the prior behaviour was to print a
// WARN, return (false, nil) from writeWorkerUnitFile, then proceed to
// StartUnit on a non-existent unit — producing a confusing podman error
// after the WARN.
func TestWorkerStartForSite_skipsLifecycleWhenUnsupported(t *testing.T) {
	registerSite(t, "ws", "/p/ws")
	fake := &stopTrackingMgr{}
	swapMgr(t, fake)

	prev := workerSupportedOnPlatform
	workerSupportedOnPlatform = func(_ config.FrameworkWorker) (bool, string) {
		return false, "host: true workers aren't supported on macOS yet"
	}
	t.Cleanup(func() { workerSupportedOnPlatform = prev })

	w := config.FrameworkWorker{Command: "npm run dev", Host: true}
	if err := WorkerStartForSite("ws", "/p/ws", "8.4", "vite", w, false); err != nil {
		t.Fatalf("expected nil error for unsupported worker, got %v", err)
	}

	if len(fake.disableCalls)+len(fake.removeServiceCalls)+len(fake.removeTimerCalls) != 0 {
		t.Errorf("expected zero lifecycle calls, got disable=%v remove=%v removeTimer=%v",
			fake.disableCalls, fake.removeServiceCalls, fake.removeTimerCalls)
	}
}

// TestWorkerStartForSite_unsupportedReasonPrinted ensures the WARN line
// surfaces the reason verbatim so users can tell *why* a worker was
// silently skipped.
func TestWorkerStartForSite_unsupportedReasonPrinted(t *testing.T) {
	registerSite(t, "ws", "/p/ws")
	swapMgr(t, &stopTrackingMgr{})

	prev := workerSupportedOnPlatform
	workerSupportedOnPlatform = func(_ config.FrameworkWorker) (bool, string) {
		return false, "fake-platform-reason"
	}
	t.Cleanup(func() { workerSupportedOnPlatform = prev })

	out := captureStdout(t, func() {
		_ = WorkerStartForSite("ws", "/p/ws", "8.4", "vite", config.FrameworkWorker{Command: "x"}, false)
	})
	if !strings.Contains(out, "fake-platform-reason") {
		t.Errorf("expected reason in WARN output, got %q", out)
	}
	if !strings.Contains(out, "⚠") {
		t.Errorf("expected warning marker in output, got %q", out)
	}
}
