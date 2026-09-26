package serviceops

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
)

// StartService is the shared start path for CLI, UI, TUI (via CLI), and MCP:
// ensure the quadlet, bring depends_on satisfiers up for customs, start the
// unit (retrying briefly for quadlet generator lag), mark it manually started,
// start reverse dependents, and regenerate dynamic_env consumers.
func StartService(name string) error {
	unit := "lerd-" + name
	if IsBuiltin(name) {
		if err := EnsureDefaultPresetQuadlet(name); err != nil {
			return err
		}
	} else {
		svc, err := config.LoadCustomService(name)
		if err != nil {
			if config.PresetExists(name) {
				return fmt.Errorf("service %q is not installed; install it with 'lerd service preset %s'", name, name)
			}
			return fmt.Errorf("unknown service %q", name)
		}
		if err := StartDependencies(svc); err != nil {
			return err
		}
		for _, engine := range sleepingAdministered(svc) {
			if err := WakeService(engine); err != nil {
				feedback.Warn("could not wake %s for %s: %v", engine, name, err)
			}
		}
		if err := EnsureCustomServiceQuadlet(svc); err != nil {
			return err
		}
	}
	if err := startUnitRetry(unit); err != nil {
		return err
	}
	_ = config.SetServicePaused(name, false)
	_ = config.SetServiceManuallyStarted(name, true)
	_ = config.SetServiceRemoved(name, false)

	// Bring up admin UIs (and similar) that declare a depends_on this service
	// can satisfy — family / env_role aware, not literal name only.
	for _, dep := range dependentsOf(name) {
		if err := EnsureServiceRunning(dep); err != nil {
			feedback.Warn("could not start dependent service %s: %v", dep, err)
		}
	}
	RegenerateDynamicEnvConsumersForService(name)
	// Starting is where the port guard shifts a service off a port something
	// else took while it was down, and where a service installed since the last
	// start first needs its dashboard served.
	syncDashboardVhost()
	return nil
}

// WakeService starts a service whose unit is already installed exactly as it
// was, dependencies first, and waits until it is ready. It skips the quadlet
// refresh EnsureServiceRunning does, which can ask a registry for a newer tag
// and hold a waiting request for seconds; a service woken from idle-suspend
// has an unchanged unit. Anything not installed goes the full way.
func WakeService(name string) error {
	return wakeService(name, map[string]bool{})
}

func wakeService(name string, seen map[string]bool) error {
	if seen[name] {
		return nil
	}
	seen[name] = true
	unit := "lerd-" + name
	if !podman.QuadletInstalled(unit) {
		return EnsureServiceRunning(name)
	}
	if svc, err := config.LoadCustomService(name); err == nil {
		for _, dep := range svc.DependsOn {
			if key := ResolveDependency(dep); key != "" {
				if err := wakeService(key, seen); err != nil {
					return fmt.Errorf("starting dependency %q for %q: %w", dep, name, err)
				}
			}
		}
		// An admin tool is no use with the engines it administers asleep.
		for _, engine := range sleepingAdministered(svc) {
			if err := wakeService(engine, seen); err != nil {
				return fmt.Errorf("waking %q for %q: %w", engine, name, err)
			}
		}
	}
	if err := wakeStartUnit(unit); err != nil {
		return err
	}
	return waitReadyFn(name, 60*time.Second)
}

// AdministeredServices returns the installed services an admin tool's
// admin_for names, matched by service name or by family, so phpMyAdmin
// covers a mariadb-11-8 as well as mysql.
func AdministeredServices(tool *config.CustomService) []string {
	if tool == nil || len(tool.AdminFor) == 0 {
		return nil
	}
	var out []string
	for _, name := range installedServiceNames() {
		if name == tool.Name {
			continue
		}
		family := config.FamilyOfName(name)
		for _, target := range tool.AdminFor {
			if target == name || (family != "" && target == family) {
				out = append(out, name)
				break
			}
		}
	}
	return out
}

// AdminToolsFor is AdministeredServices the other way round: the installed
// admin tools that administer name.
func AdminToolsFor(name string) []string {
	customs, err := config.ListCustomServices()
	if err != nil {
		return nil
	}
	var out []string
	for _, tool := range customs {
		for _, n := range AdministeredServices(tool) {
			if n == name {
				out = append(out, tool.Name)
				break
			}
		}
	}
	return out
}

// sleepingAdministered is the part of AdministeredServices idle-suspend put to
// sleep. A stopped or paused engine is the user's choice and stays down.
func sleepingAdministered(tool *config.CustomService) []string {
	var out []string
	for _, n := range AdministeredServices(tool) {
		if config.ServiceIsIdleSuspended(n) {
			out = append(out, n)
		}
	}
	return out
}

func installedServiceNames() []string {
	var out []string
	for _, n := range config.DefaultPresetNames() {
		if ServiceInstalled(n) {
			out = append(out, n)
		}
	}
	if customs, err := config.ListCustomServices(); err == nil {
		for _, c := range customs {
			out = append(out, c.Name)
		}
	}
	return out
}

// DashboardAnswers reports whether a dashboard serves HTTP yet. A running unit
// is not enough: rootless podman accepts connections on the published port
// before the app inside listens, so only a real response counts.
func DashboardAnswers(target string) bool {
	client := http.Client{
		Timeout:       time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get(target)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < http.StatusInternalServerError
}

// WaitDashboard polls DashboardAnswers until the dashboard serves or max runs out.
func WaitDashboard(target string, max time.Duration) bool {
	deadline := time.Now().Add(max)
	for !DashboardAnswers(target) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	return true
}

// wakeStartUnit is the seam WakeService starts a unit through.
var wakeStartUnit = startUnitRetry

// StopService is the shared stop path for CLI, UI, TUI (via CLI), and MCP:
// cascade-stop dependents when no other running satisfier remains, stop name,
// mark it paused, and regenerate dynamic_env consumers.
func StopService(name string) error {
	if err := StopWithDependents(name); err != nil {
		return err
	}
	_ = config.SetServicePaused(name, true)
	_ = config.SetServiceManuallyStarted(name, false)
	// A user stop outranks idle-suspend, which would otherwise wake it again.
	_ = config.SetServiceIdleSuspended(name, false)
	RegenerateDynamicEnvConsumersForService(name)
	return nil
}

// RestartService is the shared restart path for CLI, UI, TUI (via CLI), and MCP:
// refresh the quadlet (so dynamic_env and file mounts land), restart the
// unit, clear paused, and regenerate dynamic_env consumers.
func RestartService(name string) error {
	unit := "lerd-" + name
	if err := refreshServiceQuadlet(name); err != nil {
		feedback.Warn("regenerating quadlet for %s failed: %v; restarting with the existing one", name, err)
	}
	if err := podman.RestartUnit(unit); err != nil {
		return err
	}
	_ = config.SetServicePaused(name, false)
	_ = config.SetServiceManuallyStarted(name, true)
	RegenerateDynamicEnvConsumersForService(name)
	syncDashboardVhost()
	return nil
}

func refreshServiceQuadlet(name string) error {
	if IsBuiltin(name) {
		return EnsureDefaultPresetQuadlet(name)
	}
	svc, err := config.LoadCustomService(name)
	if err != nil {
		return err
	}
	return EnsureCustomServiceQuadlet(svc)
}

// startUnitRetry starts a unit, retrying briefly when the generator has not
// published the unit yet after a daemon-reload (UI and install share this).
func startUnitRetry(unit string) error {
	var err error
	for attempt := range 5 {
		err = podman.StartUnit(unit)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	return err
}
