package tui

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/config"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
)

// shortDuration drops the zero units Go's formatting keeps, so 30m0s reads 30m.
func shortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// onOffVerb is the verb that flips a toggle that is currently on or off.
func onOffVerb(on bool) string {
	if on {
		return "off"
	}
	return "on"
}

// settingsRow describes one focusable line in the settings view.
type settingsRow struct {
	kind       settingsKind
	label      string
	on         bool
	phpVersion string // PHP version, for xdebug rows
}

type settingsKind int

const (
	settingsLANExpose settingsKind = iota
	settingsLANServices
	settingsAutostart
	settingsXdebug
	settingsWorkerMode
	settingsAutoSnapshot
	settingsIdle
	settingsStreaming
	settingsTray
	settingsNotify
	settingsDNS
	settingsProfiler
)

func (m *Model) settingsRows() []settingsRow {
	cfg, _ := config.LoadGlobal()
	var rows []settingsRow

	lanExposed := cfg != nil && cfg.LAN.Exposed
	rows = append(rows, settingsRow{
		kind:  settingsLANExpose,
		label: "LAN expose (sites and DNS)",
		on:    lanExposed,
	})
	rows = append(rows, settingsRow{
		kind:  settingsLANServices,
		label: managedServiceLANLabel(cfg),
		on:    cfg != nil && cfg.LAN.ServicesExposed,
	})
	rows = append(rows, settingsRow{
		kind:  settingsAutostart,
		label: "Autostart lerd on login",
		on:    lerdSystemd.IsAutostartEnabled(),
	})
	rows = append(rows, settingsRow{
		kind:  settingsAutoSnapshot,
		label: autoSnapshotSettingLabel(cfg),
		on:    cfg.AutoSnapshotEnabled(),
	})
	idle := "Idle suspend (stop workers of sites nobody is using)"
	if cfg != nil && cfg.IdleSuspend.Enabled {
		idle = fmt.Sprintf("Idle suspend (after %s without requests)", shortDuration(cfg.IdleSuspendTimeout()))
	}
	rows = append(rows,
		settingsRow{kind: settingsIdle, label: idle, on: cfg != nil && cfg.IdleSuspend.Enabled},
		settingsRow{kind: settingsStreaming, label: "Streaming mode (hide private workspaces)", on: cfg != nil && cfg.Streaming()},
		settingsRow{kind: settingsTray, label: "Tray applet", on: cfg == nil || cfg.IsTrayEnabled()},
		settingsRow{kind: settingsNotify, label: "Notifications", on: cfg == nil || cfg.IsNotificationsEnabled()},
		settingsRow{kind: settingsDNS, label: "lerd DNS (resolves ." + currentTLD() + ")", on: !m.snap.Status.DNSDisabled},
		settingsRow{kind: settingsProfiler, label: "SPX profiler (captures slow requests)", on: cfg != nil && cfg.IsProfilerEnabled()},
	)

	// Worker runtime mode: macOS only. On Linux workers always run via
	// podman exec under systemd so the setting is meaningless there and
	// is hidden from the UI.
	if runtime.GOOS == "darwin" {
		containerMode := cfg != nil && cfg.WorkerExecMode() == config.WorkerExecModeContainer
		label := "Workers in container mode (one container per worker)"
		if !containerMode {
			label = "Workers in exec mode (lower memory, shared FPM container)"
		}
		rows = append(rows, settingsRow{
			kind:  settingsWorkerMode,
			label: label,
			on:    containerMode,
		})
	}

	// Xdebug lives in the PHP & Node view, beside the version it belongs to.
	return rows
}

func (m *Model) settingsToggle(rows []settingsRow) tea.Cmd {
	if len(rows) == 0 {
		return nil
	}
	if m.settingsRow >= len(rows) {
		m.settingsRow = len(rows) - 1
	}
	row := rows[m.settingsRow]
	switch row.kind {
	case settingsLANExpose:
		verb := "on"
		if row.on {
			verb = "off"
		}
		m.setStatus("toggling LAN expose "+verb+"…", 5*time.Second)
		return runLerd("", "lan", "expose", verb)
	case settingsLANServices:
		verb := "on"
		if row.on {
			verb = "off"
		}
		if row.on {
			m.setStatus("disabling managed service LAN access…", 5*time.Second)
		} else {
			m.setStatus("enabling managed service LAN access — trusted networks only…", 5*time.Second)
		}
		return runLerd("", "lan", "services", verb)
	case settingsAutostart:
		sub := "enable"
		if row.on {
			sub = "disable"
		}
		m.setStatus("autostart "+sub+"…", 5*time.Second)
		return runLerd("", "autostart", sub)
	case settingsXdebug:
		verb := "on"
		if row.on {
			verb = "off"
		}
		m.setStatus("xdebug "+verb+" PHP "+row.phpVersion+"…", 5*time.Second)
		return runLerd("", "xdebug", verb, row.phpVersion)
	case settingsAutoSnapshot:
		verb := "on"
		if row.on {
			verb = "off"
		}
		m.setStatus("automatic snapshots "+verb+"…", 5*time.Second)
		return runLerd("", "db:snapshot:auto", verb)
	case settingsIdle:
		m.setStatus("idle suspend "+onOffVerb(row.on)+"…", 5*time.Second)
		return runLerd("", "idle", onOffVerb(row.on))
	case settingsStreaming:
		m.setStatus("streaming mode "+onOffVerb(row.on)+"…", 5*time.Second)
		return tea.Sequence(runLerd("", "streaming", onOffVerb(row.on)), loadCmd())
	case settingsTray:
		m.setStatus("tray applet "+onOffVerb(row.on)+"…", 5*time.Second)
		return runLerd("", "tray", onOffVerb(row.on))
	case settingsNotify:
		m.setStatus("notifications "+onOffVerb(row.on)+"…", 5*time.Second)
		return runLerd("", "notify", onOffVerb(row.on))
	case settingsDNS:
		verb := "dns:disable"
		if !row.on {
			verb = "dns:enable"
		}
		m.setStatus("lerd DNS "+onOffVerb(row.on)+"…", 10*time.Second)
		return tea.Sequence(runLerd("", verb), loadCmd())
	case settingsProfiler:
		m.setStatus("profiler "+onOffVerb(row.on)+"…", 5*time.Second)
		return runLerd("", "profile", onOffVerb(row.on))
	case settingsWorkerMode:
		// Toggle between exec (off) and container (on). Mirrors
		// `lerd workers mode <value>`. Does not stop running workers —
		// caller should restart them for the change to take effect.
		target := config.WorkerExecModeContainer
		if row.on {
			target = config.WorkerExecModeExec
		}
		m.setStatus("switching worker mode to "+target+"…", 5*time.Second)
		return runLerd("", "workers", "mode", target)
	}
	return nil
}

// autoSnapshotSettingLabel names the schedule on the row, so the cadence and the
// retention are readable without opening anything.
func autoSnapshotSettingLabel(cfg *config.GlobalConfig) string {
	if !cfg.AutoSnapshotEnabled() {
		return "Automatic database snapshots"
	}
	scope := "all sites"
	if cfg.AutoSnapshotSelection() == config.AutoSnapshotOptIn {
		scope = "opted-in sites"
	}
	return fmt.Sprintf("Automatic database snapshots (every %s, keeping %d, %s)",
		compactEvery(cfg.AutoSnapshotEvery()), cfg.AutoSnapshotKeep(), scope)
}

// compactEvery drops the empty trailing units Go leaves on a whole-hour duration.
func compactEvery(d time.Duration) string {
	out := d.String()
	if strings.HasSuffix(out, "m0s") {
		out = strings.TrimSuffix(out, "0s")
	}
	if strings.HasSuffix(out, "h0m") {
		out = strings.TrimSuffix(out, "0m")
	}
	return out
}

func managedServiceLANLabel(cfg *config.GlobalConfig) string {
	if cfg != nil && cfg.LAN.ServicesExposed && !cfg.LAN.Exposed {
		return "Managed service LAN access (inactive — LAN exposure off)"
	}
	return "Managed service LAN access"
}
