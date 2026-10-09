//go:build windows

package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolateWinData(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func sleeperUnit(t *testing.T) string {
	t.Helper()
	ps, err := findPowerShell()
	if err != nil {
		t.Skip("powershell not found")
	}
	return "[Service]\nExecStart=" + ps + " -NoProfile -Command Start-Sleep 60\nRestart=on-failure\n"
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 50; i++ {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestWinServiceUnitLifecycle(t *testing.T) {
	isolateWinData(t)
	m := &windowsServiceManager{}
	name := "lerd-test-sleeper"

	if got, _ := m.UnitStatus(name); got != "unknown" {
		t.Fatalf("status of a never-written unit = %q, want unknown", got)
	}
	if err := m.WriteServiceUnit(name, sleeperUnit(t)); err != nil {
		t.Fatal(err)
	}
	if !m.IsEnabled(name) {
		t.Error("a written unit should count as enabled")
	}
	if got := m.ListServiceUnits("lerd-test-*"); len(got) != 1 || got[0] != name {
		t.Errorf("ListServiceUnits = %v", got)
	}
	if got, _ := m.UnitStatus(name); got != "inactive" {
		t.Errorf("status before start = %q, want inactive", got)
	}

	if err := m.Start(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(name) })
	waitFor(t, "unit active", func() bool { return m.IsActive(name) })
	// A unit that dies just after starting must not count as running.
	time.Sleep(1500 * time.Millisecond)
	if !m.IsActive(name) {
		t.Fatal("unit was active at start but exited within 1.5s")
	}
	if got, _ := m.UnitStatus(name); got != "active" {
		t.Errorf("status while running = %q, want active", got)
	}
	if st := m.AllUnitStates(); st[name] != "active" || st[name+".service"] != "active" {
		t.Errorf("AllUnitStates = %v", st)
	}

	if err := m.Stop(name); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "unit stopped", func() bool { return !m.IsActive(name) })
	if got, _ := m.UnitStatus(name); got != "inactive" {
		t.Errorf("status after clean stop = %q, want inactive", got)
	}

	if err := m.RemoveServiceUnit(name); err != nil {
		t.Fatal(err)
	}
	if got := m.ListServiceUnits("lerd-test-*"); len(got) != 0 {
		t.Errorf("unit still listed after removal: %v", got)
	}
}

func TestWinCrashedUnitReportsFailed(t *testing.T) {
	isolateWinData(t)
	m := &windowsServiceManager{}
	name := "lerd-test-crash"
	if err := m.WriteServiceUnit(name, sleeperUnit(t)); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(name); err != nil {
		t.Fatal(err)
	}
	pid := readPID(name)
	if pid == 0 {
		t.Fatal("no pid recorded")
	}
	killTree(pid) // dies without going through Stop
	waitFor(t, "process gone", func() bool { return !pidAlive(pid) })
	if got, _ := m.UnitStatus(name); got != "failed" {
		t.Errorf("status after an unrequested exit = %q, want failed", got)
	}
}

func TestWinWriteServiceUnitIfChanged(t *testing.T) {
	isolateWinData(t)
	m := &windowsServiceManager{}
	unit := sleeperUnit(t)
	if changed, err := m.WriteServiceUnitIfChanged("lerd-test-x", unit); err != nil || !changed {
		t.Fatalf("first write: changed=%v err=%v", changed, err)
	}
	if changed, _ := m.WriteServiceUnitIfChanged("lerd-test-x", unit); changed {
		t.Error("identical content must not report a change")
	}
}

// lerd-tray reads LERD_TRAY_DAEMON=1 from its unit to stay in the foreground;
// dropped, it detached and the unit read inactive while the tray ran on.
func TestWinServiceUnitKeepsItsEnvironment(t *testing.T) {
	isolateWinData(t)
	m := &windowsServiceManager{}
	unit := sleeperUnit(t) + "Environment=LERD_TRAY_DAEMON=1\nEnvironment=\"GREETING=hello world\"\n"
	if err := m.WriteServiceUnit("lerd-test-env", unit); err != nil {
		t.Fatal(err)
	}
	d, err := loadDef("lerd-test-env")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"LERD_TRAY_DAEMON=1", "GREETING=hello world"}; strings.Join(d.Env, "|") != strings.Join(want, "|") {
		t.Errorf("Env = %q, want %q", d.Env, want)
	}

	if err := m.WriteServiceUnit("lerd-test-noenv", sleeperUnit(t)); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(defPath("lerd-test-noenv")); strings.Contains(string(data), `"env"`) {
		t.Errorf("a unit with no Environment= should not grow an env key:\n%s", data)
	}
}

func TestWinSupervisedUnitRunsWithItsEnvironment(t *testing.T) {
	isolateWinData(t)
	ps, err := findPowerShell()
	if err != nil {
		t.Skip("powershell not found")
	}
	out := filepath.Join(t.TempDir(), "env.txt")
	m := &windowsServiceManager{}
	name := "lerd-test-envrun"
	unit := "[Service]\nExecStart=" + ps + " -NoProfile -Command \"Set-Content -Path '" + out + "' -Value $env:LERD_TEST_ENV; Start-Sleep 60\"\n" +
		"Restart=on-failure\nEnvironment=LERD_TEST_ENV=from-the-unit\n"
	if err := m.WriteServiceUnit(name, unit); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(name) })
	waitFor(t, "the unit to write its environment", func() bool {
		data, err := os.ReadFile(out)
		return err == nil && strings.TrimSpace(string(data)) != ""
	})
	if data, _ := os.ReadFile(out); strings.TrimSpace(string(data)) != "from-the-unit" {
		t.Errorf("unit saw LERD_TEST_ENV=%q, want from-the-unit", strings.TrimSpace(string(data)))
	}
}

func TestWinContainerUnitStoresPodmanArgs(t *testing.T) {
	isolateWinData(t)
	m := &windowsServiceManager{}
	content := "[Container]\nImage=docker.io/library/redis:7\nContainerName=lerd-redis\n" +
		"Volume=C:\\data\\redis:/data:z\nPublishPort=127.0.0.1:6379:6379\n"
	if err := m.WriteContainerUnit("lerd-redis", content); err != nil {
		t.Fatal(err)
	}
	if !m.ContainerUnitInstalled("lerd-redis") {
		t.Fatal("container unit not installed")
	}
	def, err := loadDef("lerd-redis")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(def.Args, " ")
	for _, want := range []string{"run -d", "--name lerd-redis", `-v C:\data\redis:/data`, "docker.io/library/redis:7"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, ":z") {
		t.Errorf("SELinux flag leaked into args: %s", joined)
	}
	if _, err := os.Stat(filepath.Join(unitsDir(), "lerd-redis.json")); err != nil {
		t.Error(err)
	}
	if err := m.RemoveContainerUnit("lerd-redis"); err != nil {
		t.Fatal(err)
	}
	if m.ContainerUnitInstalled("lerd-redis") {
		t.Error("still installed after removal")
	}
}

// Starting a container unit that is already running must leave it alone, the
// way systemctl start does on Linux. podman run --replace recreated every
// container, databases included, on each lerd start.
func TestWinStartLeavesARunningContainerAlone(t *testing.T) {
	isolateWinData(t)
	m := &windowsServiceManager{}
	if err := m.WriteContainerUnit("lerd-redis", "[Container]\nImage=docker.io/library/redis:7\nContainerName=lerd-redis\n"); err != nil {
		t.Fatal(err)
	}
	prevRunning, prevRun := containerRunning, runContainer
	t.Cleanup(func() { containerRunning, runContainer = prevRunning, prevRun })
	runs := 0
	runContainer = func([]string) error { runs++; return nil }

	containerRunning = func(string) (bool, error) { return true, nil }
	if err := m.Start("lerd-redis"); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Errorf("a running container was run again")
	}

	containerRunning = func(string) (bool, error) { return false, nil }
	if err := m.Start("lerd-redis"); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Errorf("a stopped container was not started, runs = %d", runs)
	}
}
