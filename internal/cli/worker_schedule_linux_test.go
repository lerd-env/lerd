//go:build linux

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteWorkerUnitFile_ScheduleEmitsTimer asserts that providing a
// non-empty schedule produces both a Type=oneshot service and a sibling
// .timer with the right OnCalendar expression — the shape Laravel <=10
// scheduler needs to avoid the Restart=always loop bug.
func TestWriteWorkerUnitFile_ScheduleEmitsTimer(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	changed, err := writeWorkerUnitFile(
		"lerd-schedule-mysite", "Task Scheduler", "mysite",
		"/srv/mysite", "8.3", "php artisan schedule:run",
		"always", "minutely", "lerd-php83-fpm", "", false,
	)
	if err != nil {
		t.Fatalf("writeWorkerUnitFile: %v", err)
	}
	if !changed {
		t.Errorf("first write reported changed=false, want true")
	}

	systemdDir := filepath.Join(tmp, "systemd", "user")
	svc, err := os.ReadFile(filepath.Join(systemdDir, "lerd-schedule-mysite.service"))
	if err != nil {
		t.Fatalf("read service: %v", err)
	}
	if !strings.Contains(string(svc), "Type=oneshot") {
		t.Errorf("scheduled worker service missing Type=oneshot:\n%s", svc)
	}
	if strings.Contains(string(svc), "Restart=") {
		t.Errorf("scheduled worker service must not declare Restart=:\n%s", svc)
	}

	timer, err := os.ReadFile(filepath.Join(systemdDir, "lerd-schedule-mysite.timer"))
	if err != nil {
		t.Fatalf("read timer: %v", err)
	}
	if !strings.Contains(string(timer), "OnCalendar=minutely") {
		t.Errorf("timer missing OnCalendar=minutely:\n%s", timer)
	}
	if !strings.Contains(string(timer), "WantedBy=timers.target") {
		t.Errorf("timer missing WantedBy=timers.target:\n%s", timer)
	}
}

// TestWriteWorkerUnitFile_DaemonRemovesStaleTimer asserts that switching
// a worker from scheduled back to daemon shape (e.g. user removes the
// schedule field from the framework yaml) cleans up the lingering
// .timer file so systemd doesn't keep firing the old oneshot.
func TestWriteWorkerUnitFile_DaemonRemovesStaleTimer(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if _, err := writeWorkerUnitFile(
		"lerd-queue-mysite", "Queue Worker", "mysite",
		"/srv/mysite", "8.3", "php artisan queue:work",
		"always", "minutely", "lerd-php83-fpm", "", false,
	); err != nil {
		t.Fatalf("seed scheduled: %v", err)
	}

	timerPath := filepath.Join(tmp, "systemd", "user", "lerd-queue-mysite.timer")
	if _, err := os.Stat(timerPath); err != nil {
		t.Fatalf("stale timer not seeded: %v", err)
	}

	if _, err := writeWorkerUnitFile(
		"lerd-queue-mysite", "Queue Worker", "mysite",
		"/srv/mysite", "8.3", "php artisan queue:work",
		"always", "", "lerd-php83-fpm", "", false,
	); err != nil {
		t.Fatalf("rewrite as daemon: %v", err)
	}

	if _, err := os.Stat(timerPath); !os.IsNotExist(err) {
		t.Errorf("stale .timer still present after switching to daemon shape: %v", err)
	}

	svc, err := os.ReadFile(filepath.Join(tmp, "systemd", "user", "lerd-queue-mysite.service"))
	if err != nil {
		t.Fatalf("read service: %v", err)
	}
	if !strings.Contains(string(svc), "Restart=always") {
		t.Errorf("daemon worker service missing Restart=always:\n%s", svc)
	}
}

// A schedule expression this host's systemd cannot parse makes the .timer unit
// refuse to load outright, so the scheduled worker silently stops existing.
// Refuse to write it, and leave no half-built pair behind.
func TestWriteWorkerUnitFile_UnparseableScheduleRefuses(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	orig := calendarValidFn
	calendarValidFn = func(string) bool { return false }
	defer func() { calendarValidFn = orig }()

	_, err := writeWorkerUnitFile(
		"lerd-schedule-mysite", "Task Scheduler", "mysite",
		"/srv/mysite", "8.3", "php artisan schedule:run",
		"always", "every-other-tuesday", "lerd-php83-fpm", "", false,
	)
	if err == nil {
		t.Fatal("expected an unparseable schedule to be refused")
	}
	if !strings.Contains(err.Error(), "every-other-tuesday") {
		t.Errorf("error should name the offending expression, got %q", err)
	}
	systemdDir := filepath.Join(tmp, "systemd", "user")
	if _, statErr := os.Stat(filepath.Join(systemdDir, "lerd-schedule-mysite.timer")); statErr == nil {
		t.Error("a refused schedule must not leave a timer unit behind")
	}
	if _, statErr := os.Stat(filepath.Join(systemdDir, "lerd-schedule-mysite.service")); statErr == nil {
		t.Error("a refused schedule must not leave a service unit behind")
	}
}

// The real validator answers for this host's systemd, which is the version-skew
// question worth asking. Skipped where systemd-analyze is not installed.
func TestCalendarValid_UsesHostSystemd(t *testing.T) {
	if !systemdAnalyzeAvailable() {
		t.Skip("systemd-analyze not installed")
	}
	if !calendarValid("minutely") {
		t.Error("minutely should be accepted by systemd-analyze")
	}
	if calendarValid("every-other-tuesday") {
		t.Error("every-other-tuesday should be rejected by systemd-analyze")
	}
}
