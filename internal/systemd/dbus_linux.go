//go:build linux

package systemd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/coreos/go-systemd/v22/dbus"
	godbus "github.com/godbus/dbus/v5"
)

// errUnitOpTimedOut is the sentinel a single unit-op attempt returns when the
// job channel does not report a result within the per-attempt deadline. It is
// wrapped into the caller-facing message by unitOpTimeoutError.
var errUnitOpTimedOut = errors.New("unit op timed out")

// jobWaitFloor is the shortest a unit op waits for systemd to report its job
// result, and the whole wait for anything that declares no window of its own.
const jobWaitFloor = 30 * time.Second

// jobWaitMargin is added to a declared stop window so the wait outlasts the
// SIGKILL systemd sends at the end of it, leaving room for the container to be
// reaped and the job result to arrive.
const jobWaitMargin = 10 * time.Second

// stopJobWait is how long a stop waits for its job result, given the window the
// unit declares as TimeoutStopSec. A service that asks for a long graceful
// shutdown is guaranteed to outlast a fixed wait, and the caller then acts on a
// "stopped" that has not happened yet, so the wait is read from the unit. A
// unit that declares nothing, or less than the floor, keeps the historic 30s.
func stopJobWait(declared time.Duration) time.Duration {
	if w := declared + jobWaitMargin; declared > 0 && w > jobWaitFloor {
		return w
	}
	return jobWaitFloor
}

// unitOpJobWait is how long any unit op waits for its job result. A stop has to
// outlast the window the unit declares, and so does a start or a restart: both
// are queued behind a stop that is already in flight, so they wait out that stop
// before their own work begins. Giving them the bare floor made every db command
// issued during a slow service's restart fail with "start … timed out after 30s"
// while the service itself came up in a second once its turn arrived.
func unitOpJobWait(op string, declaredStop time.Duration) time.Duration {
	switch op {
	case "stop", "start", "restart":
		return stopJobWait(declaredStop)
	default:
		return jobWaitFloor
	}
}

// unitOpTimeoutError builds the caller-facing timeout message, reporting the
// wait that actually elapsed rather than the fixed number it used to name.
func unitOpTimeoutError(verb, name string, wait time.Duration) error {
	return fmt.Errorf("%s %s timed out after %s", verb, name, wait)
}

// declaredStopTimeout reads the unit's own TimeoutStopSec, which quadlet
// generation sets from the service definition. Anything unreadable, absent or
// infinite reports zero, which stopJobWait resolves to the floor.
func declaredStopTimeout(conn *dbus.Conn, unit string) time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	prop, err := conn.GetUnitTypePropertyContext(ctx, unit, "Service", "TimeoutStopUSec")
	if err != nil || prop == nil {
		return 0
	}
	usec, ok := prop.Value.Value().(uint64)
	if !ok || usec == 0 || usec == math.MaxUint64 {
		return 0
	}
	return time.Duration(usec) * time.Microsecond
}

// stopRetryAttempts bounds how many times a "stop" job is re-issued when
// systemd reports the result "canceled". `lerd stop` deactivates many units
// in parallel with mode "replace"; stopping a unit that other units BindsTo
// (e.g. the PHP-FPM unit, which the per-site worker units bind to) enqueues a
// dependency-driven stop that a competing explicit StopUnit replaces, so one
// of the two jobs comes back "canceled" and its container is left running. A
// fresh stop issued once the competing transaction has settled completes
// cleanly; only "stop" is retried (start/restart keep their single-attempt
// behaviour).
const stopRetryAttempts = 4

// userBus holds the lazily-initialised systemd user bus connection. Long-lived:
// the library handles reconnection internally and the process lifetime is the
// natural owner. sync.Once guards the first-dial race.
var (
	userBusOnce sync.Once
	userBusConn *dbus.Conn
	userBusErr  error
)

func userConn() (*dbus.Conn, error) {
	userBusOnce.Do(func() {
		// Dial context must not be cancellable: go-systemd ties the
		// conn lifetime to this ctx, so cancelling it invalidates the
		// underlying socket for every subsequent op in this process.
		userBusConn, userBusErr = dbus.NewUserConnectionContext(context.Background())
	})
	return userBusConn, userBusErr
}

// dbusUnitOp runs one of StartUnit / StopUnit / RestartUnit / ReloadUnit by
// name and waits for systemd to report the result on the internal channel.
// Returns an error whose message mirrors the old systemctl shell-out for
// drop-in compatibility with existing error strings.
func dbusUnitOp(op, verb, name string) error {
	conn, err := userConn()
	if err != nil {
		return fmt.Errorf("%s %s: dbus connect: %w", verb, name, err)
	}
	unit := withServiceSuffix(name)

	wait := unitOpJobWait(op, declaredStopTimeout(conn, unit))

	// attempt enqueues one job and waits for systemd to report its result,
	// returning the result string ("done", "canceled", "failed", …) or a
	// transport/timeout error.
	attempt := func() (string, error) {
		ch := make(chan string, 1)
		ctx, cancel := context.WithTimeout(context.Background(), wait)
		defer cancel()
		var opErr error
		switch op {
		case "start":
			_, opErr = conn.StartUnitContext(ctx, unit, "replace", ch)
		case "stop":
			_, opErr = conn.StopUnitContext(ctx, unit, "replace", ch)
		case "restart":
			_, opErr = conn.RestartUnitContext(ctx, unit, "replace", ch)
		default:
			return "", fmt.Errorf("unknown unit op %q", op)
		}
		if opErr != nil {
			return "", opErr
		}
		select {
		case result := <-ch:
			return result, nil
		case <-ctx.Done():
			return "", errUnitOpTimedOut
		}
	}

	maxAttempts := 1
	if op == "stop" {
		maxAttempts = stopRetryAttempts
	}
	result, err := runUnitOpWithRetry(maxAttempts, settleBetweenStops, attempt)
	if err != nil {
		if errors.Is(err, errUnitOpTimedOut) {
			return unitOpTimeoutError(verb, name, wait)
		}
		return fmt.Errorf("%s %s failed: %w", verb, name, err)
	}
	if result != "done" {
		return fmt.Errorf("%s %s failed: %s%s", verb, name, result, unitFailureDetail(conn, name))
	}
	return nil
}

// runUnitOpWithRetry runs do() up to maxAttempts times, re-issuing the job
// only while systemd reports the retryable result "canceled" (see
// stopRetryAttempts). It returns the last result string and any transport
// error from do. settle, when non-nil, is called between attempts to let a
// competing transaction settle before retrying. Pure of any DBus dependency
// so the retry policy is unit-testable.
func runUnitOpWithRetry(maxAttempts int, settle func(attempt int), do func() (string, error)) (string, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var result string
	for attempt := 1; ; attempt++ {
		var err error
		result, err = do()
		if err != nil {
			return result, err
		}
		if result == "done" || result != "canceled" || attempt >= maxAttempts {
			return result, nil
		}
		if settle != nil {
			settle(attempt)
		}
	}
}

// settleBetweenStops backs off briefly (and proportionally to the attempt)
// before re-issuing a canceled stop, giving systemd time to drain the
// competing job that caused the cancellation.
func settleBetweenStops(attempt int) {
	time.Sleep(time.Duration(attempt) * 150 * time.Millisecond)
}

// DBusDaemonReload runs systemctl --user daemon-reload over DBus.
func DBusDaemonReload() error {
	conn, err := userConn()
	if err != nil {
		return fmt.Errorf("daemon-reload: dbus connect: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := conn.ReloadContext(ctx); err != nil {
		return fmt.Errorf("daemon-reload failed: %w", err)
	}
	return nil
}

// DBusStartUnit starts a user unit via DBus and waits for the job to finish.
func DBusStartUnit(name string) error {
	_ = DBusResetFailed(name)
	return dbusUnitOp("start", "start", name)
}

// DBusStopUnit stops a user unit via DBus.
func DBusStopUnit(name string) error {
	if err := dbusUnitOp("stop", "stop", name); err != nil {
		if unitNotLoaded(err) {
			return nil
		}
		return err
	}
	_ = DBusResetFailed(name)
	return nil
}

// unitNotLoaded reports whether an op failed because systemd has no such unit.
// For a stop that is the goal already met, not a failure: the stop set is built
// partly from what a site declares rather than from what is on disk, so a
// machine that never installed a declared worker named units systemd has never
// heard of and a clean shutdown printed a failure for each one. launchd's side
// has always treated its equivalent (exit 36) as success.
func unitNotLoaded(err error) bool {
	var dberr godbus.Error
	if errors.As(err, &dberr) {
		return dberr.Name == "org.freedesktop.systemd1.NoSuchUnit"
	}
	return false
}

// DBusRestartUnit restarts a user unit via DBus.
func DBusRestartUnit(name string) error {
	return dbusUnitOp("restart", "restart", name)
}

// DBusResetFailed clears any "failed" state for the named unit so the next
// start is not blocked by Restart= rate-limits.
func DBusResetFailed(name string) error {
	conn, err := userConn()
	if err != nil {
		return err
	}
	return conn.ResetFailedUnitContext(context.Background(), withServiceSuffix(name))
}

// DBusEnableService marks a user service to start at login.
func DBusEnableService(name string) error {
	conn, err := userConn()
	if err != nil {
		return fmt.Errorf("enable %s: dbus connect: %w", name, err)
	}
	_, _, err = conn.EnableUnitFilesContext(
		context.Background(),
		[]string{withServiceSuffix(name)},
		false, true,
	)
	if err != nil {
		return fmt.Errorf("enable %s: %w", name, err)
	}
	return nil
}

// DBusDisableService removes a user service from the login start set.
func DBusDisableService(name string) error {
	conn, err := userConn()
	if err != nil {
		return fmt.Errorf("disable %s: dbus connect: %w", name, err)
	}
	if _, err := conn.DisableUnitFilesContext(
		context.Background(),
		[]string{withServiceSuffix(name)},
		false,
	); err != nil {
		return fmt.Errorf("disable %s: %w", name, err)
	}
	return nil
}

// DBusActiveState returns the ActiveState property ("active", "inactive",
// "failed", "activating", …) for the named unit, or "" when the unit is
// unknown. Unit name may be bare (e.g. "lerd-foo") or fully-qualified
// ("lerd-foo.service", "lerd-foo.timer").
func DBusActiveState(name string) string {
	conn, err := userConn()
	if err != nil {
		return ""
	}
	props, err := conn.GetUnitPropertiesContext(context.Background(), withDefaultSuffix(name))
	if err != nil {
		return ""
	}
	s, _ := props["ActiveState"].(string)
	return s
}

// DBusIsEnabled returns true when the unit-file state resolves to "enabled".
func DBusIsEnabled(name string) bool {
	conn, err := userConn()
	if err != nil {
		return false
	}
	props, err := conn.GetUnitPropertiesContext(context.Background(), withServiceSuffix(name))
	if err != nil {
		return false
	}
	s, _ := props["UnitFileState"].(string)
	return s == "enabled"
}

// withServiceSuffix ensures the unit name ends in ".service" which DBus
// requires for enable/disable and for unit-property lookups. Bare names are
// what callers pass today when they shell out to systemctl.
func withServiceSuffix(name string) string {
	if strings.Contains(name, ".") {
		return name
	}
	return name + ".service"
}

// withDefaultSuffix keeps an explicit .timer / .service suffix when the
// caller passed one, and otherwise assumes .service. Used by property
// lookups where a bare name could legitimately refer to either unit type.
func withDefaultSuffix(name string) string {
	if strings.Contains(name, ".") {
		return name
	}
	return name + ".service"
}

// NotifyReady tells systemd the current process has finished its startup
// work and is ready to serve. Used by Type=notify units so systemctl start
// blocks until the service is actually up, not just spawned. No-op outside
// a systemd-managed process (returns false without error).
func NotifyReady() {
	_, _ = daemon.SdNotify(false, daemon.SdNotifyReady)
}

// NotifyStopping tells systemd the process is winding down, letting
// dependent units start their own teardown early instead of waiting for
// the process to actually exit.
func NotifyStopping() {
	_, _ = daemon.SdNotify(false, daemon.SdNotifyStopping)
}
