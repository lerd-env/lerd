//go:build !nogui

package tray

import (
	"fmt"
	"runtime"

	"github.com/getlantern/systray"
)

const (
	maxServices = 20
	maxPHP      = 8
)

type menuState struct {
	mStatus  *systray.MenuItem
	mNginx   *systray.MenuItem
	mDNS     *systray.MenuItem
	mWorkers *systray.MenuItem

	mDash   *systray.MenuItem
	mToggle *systray.MenuItem

	mSvcs *systray.MenuItem
	svcs  *menuList

	mPHP *systray.MenuItem
	php  *menuList

	mSettings      *systray.MenuItem
	mAutostart     *systray.MenuItem
	mLAN           *systray.MenuItem
	mLANServices   *systray.MenuItem
	mDumps         *systray.MenuItem
	mNotifications *systray.MenuItem
	mIconStyle     *systray.MenuItem
	mUpdate        *systray.MenuItem
	mQuit          *systray.MenuItem
}

func buildMenu(mono bool) *menuState {
	m := &menuState{}

	m.mStatus = systray.AddMenuItem("⏳ Checking...", "")
	m.mStatus.Disable()
	m.mNginx = systray.AddMenuItem("  🔴 nginx", "")
	m.mNginx.Disable()
	m.mDNS = systray.AddMenuItem("  🔴 dns", "")
	m.mDNS.Disable()
	m.mWorkers = systray.AddMenuItem("", "")
	m.mWorkers.Disable()
	m.mWorkers.Hide()

	systray.AddSeparator()

	m.mDash = systray.AddMenuItem("Open Dashboard", "Open the Lerd web dashboard")
	m.mToggle = systray.AddMenuItem("Start Lerd", "Start or stop the Lerd environment")

	systray.AddSeparator()

	m.mSvcs = systray.AddMenuItem("Services", "Start or stop a service")
	m.svcs = newMenuList(m.mSvcs, maxServices)
	m.mPHP = systray.AddMenuItem("PHP", "Switch the default PHP version")
	m.php = newMenuList(m.mPHP, maxPHP)

	systray.AddSeparator()

	m.mSettings = systray.AddMenuItem("Settings", "Lerd settings")
	m.mAutostart = m.mSettings.AddSubMenuItem("Autostart at login: Off", "Toggle lerd autostart on login")
	// LAN exposure toggle is not shown on macOS — ports 80/443 are always
	// reachable on the LAN via gvproxy; only non-privileged ports support IP binding.
	if runtime.GOOS != "darwin" {
		m.mLAN = m.mSettings.AddSubMenuItem("Expose to LAN: Off", "Toggle whether lerd is reachable from other devices on the local network")
	}
	m.mLANServices = m.mSettings.AddSubMenuItem("Managed service LAN access: Off", "Allow remote access to managed service ports on trusted networks")
	m.mDumps = m.mSettings.AddSubMenuItem("Debug bridge: Off", "Capture dump() / dd() into the lerd dashboard")
	m.mNotifications = m.mSettings.AddSubMenuItem("Notifications: On", "Globally enable or disable lerd notifications")
	// The high-contrast icon toggle only makes sense for the colour icon; in
	// mono mode the OS recolors the template icon to match the panel itself.
	if !mono {
		m.mIconStyle = m.mSettings.AddSubMenuItem("High-contrast icon: Off", "Show an always-visible green running icon instead of the theme-adaptive one")
	}

	m.mUpdate = systray.AddMenuItem("Check for update...", "Check for a newer version of Lerd")
	m.mQuit = systray.AddMenuItem("Quit Lerd", "Stop all Lerd processes and containers")

	return m
}

// serviceRows renders one row per service. Paused services show yellow: they
// were stopped deliberately, so red would read as broken.
func serviceRows(snap *Snapshot) []row {
	rows := make([]row, 0, len(snap.Services))
	for _, svc := range snap.Services {
		dot, verb := "🔴", "start"
		switch {
		case svc.Paused:
			dot = "🟡"
		case svc.Status == "active":
			dot, verb = "🟢", "stop"
		}
		rows = append(rows, row{
			title:   fmt.Sprintf("%s %s", dot, svc.Name),
			tooltip: fmt.Sprintf("Click to %s %s", verb, svc.Name),
			args:    []string{"service", verb, svc.Name},
		})
	}
	return rows
}

// phpRows renders the installed PHP versions, ticking the current default.
func phpRows(snap *Snapshot) []row {
	rows := make([]row, 0, len(snap.PHPVersions))
	for _, p := range snap.PHPVersions {
		title := p.Version
		if p.Version == snap.PHPDefault {
			title = "✔ " + p.Version
		}
		rows = append(rows, row{
			title:   title,
			tooltip: "Make PHP " + p.Version + " the default",
			args:    []string{"use", p.Version},
		})
	}
	return rows
}

// servicesTitle summarises the section on the parent row so the count reads
// without opening the submenu.
func servicesTitle(snap *Snapshot) string {
	if len(snap.Services) == 0 {
		return "Services"
	}
	running := 0
	for _, svc := range snap.Services {
		if svc.Status == "active" && !svc.Paused {
			running++
		}
	}
	return fmt.Sprintf("Services (%d/%d)", running, len(snap.Services))
}

func phpTitle(snap *Snapshot) string {
	if snap.PHPDefault == "" {
		return "PHP"
	}
	return "PHP " + snap.PHPDefault
}

// workersTitle renders the worker line and reports whether it should show at
// all. A stopped worker is normal on a paused or idle-suspended site, so the
// alarm comes from the health endpoint's verdict, never from a raw count; with
// nothing running and nothing broken the line stays hidden rather than
// standing permanently red.
func workersTitle(snap *Snapshot) (string, bool) {
	switch down := len(snap.WorkersDown); {
	case down == 1:
		return fmt.Sprintf("  🔴 worker down (%s)", snap.WorkersDown[0]), true
	case down > 1:
		return fmt.Sprintf("  🔴 %d workers down", down), true
	case snap.WorkersRunning == 1:
		return "  🟢 1 worker", true
	case snap.WorkersRunning > 1:
		return fmt.Sprintf("  🟢 %d workers", snap.WorkersRunning), true
	default:
		return "", false
	}
}

func statusDot(ok bool) string {
	if ok {
		return "🟢"
	}
	return "🔴"
}

// toggleTitle renders a settings toggle, e.g. "Notifications: ✔ On".
func toggleTitle(label string, on bool) string {
	if on {
		return label + ": ✔ On"
	}
	return label + ": Off"
}

func managedServiceLANTitle(snap *Snapshot) string {
	if !snap.LANServicesExposed {
		return "Managed service LAN access: Off"
	}
	if !snap.LANExposed {
		return "Managed service LAN access: Armed (LAN exposure off)"
	}
	return "Managed service LAN access: ✔ On"
}

// apply updates menu titles and visibility from a Snapshot.
func (m *menuState) apply(snap *Snapshot) {
	if snap == nil {
		snap = &Snapshot{}
	}

	if snap.Running {
		m.mStatus.SetTitle("🟢 Running")
		m.mToggle.SetTitle("Stop Lerd")
	} else {
		m.mStatus.SetTitle("🔴 Stopped")
		m.mToggle.SetTitle("Start Lerd")
	}

	m.mNginx.SetTitle(fmt.Sprintf("  %s nginx", statusDot(snap.NginxRunning)))

	switch {
	case snap.DNSDisabled:
		m.mDNS.SetTitle("  ⚪ dns (disabled)")
	case !snap.DNSOK && snap.DNSDegraded:
		m.mDNS.SetTitle("  🟡 dns")
	default:
		m.mDNS.SetTitle(fmt.Sprintf("  %s dns", statusDot(snap.DNSOK)))
	}

	if title, show := workersTitle(snap); show {
		m.mWorkers.SetTitle(title)
		m.mWorkers.Show()
	} else {
		m.mWorkers.Hide()
	}

	if m.svcs.set(serviceRows(snap)) == 0 {
		m.mSvcs.Hide()
	} else {
		m.mSvcs.SetTitle(servicesTitle(snap))
		m.mSvcs.Show()
	}

	if m.php.set(phpRows(snap)) == 0 {
		m.mPHP.Hide()
	} else {
		m.mPHP.SetTitle(phpTitle(snap))
		m.mPHP.Show()
	}

	m.mAutostart.SetTitle(toggleTitle("Autostart at login", snap.AutostartEnabled))
	if m.mLAN != nil {
		m.mLAN.SetTitle(toggleTitle("Expose to LAN", snap.LANExposed))
	}
	m.mLANServices.SetTitle(managedServiceLANTitle(snap))
	m.mDumps.SetTitle(toggleTitle("Debug bridge", snap.DumpsEnabled))
	m.mNotifications.SetTitle(toggleTitle("Notifications", snap.NotificationsEnabled))
	if m.mIconStyle != nil {
		m.mIconStyle.SetTitle(toggleTitle("High-contrast icon", snap.HighContrastIcon))
	}

	if snap.LatestVersion != "" {
		m.mUpdate.SetTitle(fmt.Sprintf("⬆ Update to %s", snap.LatestVersion))
	} else {
		m.mUpdate.SetTitle("Check for update...")
	}
}
