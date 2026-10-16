package tray

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Snapshot holds the state polled from the Lerd API.
type Snapshot struct {
	Running              bool
	NginxRunning         bool
	DNSOK                bool
	DNSDegraded          bool // lerd-dns healthy but the system resolver is bypassed, typically a VPN
	DNSDisabled          bool // explicit dns.enabled=false from API; zero value falls through to ok/error
	PHPVersions          []phpInfo
	PHPDefault           string
	Services             []serviceInfo
	ServicesKnown        bool     // the services call answered; an empty list means none, not unread
	WorkersRunning       int      // per-site worker units currently up
	WorkersDown          []string // sites lerd considers unhealthy, not merely stopped
	AutostartEnabled     bool
	LANExposed           bool   // lerd lan expose state — drives the LAN toggle item
	LANServicesExposed   bool   // explicit managed-service LAN access preference
	DumpsEnabled         bool   // lerd dump on/off state — drives the dump toggle item
	NotificationsEnabled bool   // lerd notify on/off state — drives the notifications toggle item
	HighContrastIcon     bool   // lerd tray icon high-contrast state — green running icon on any panel
	LatestVersion        string // non-empty (e.g. "v0.8.5") when a newer version is available
}

type phpInfo struct {
	Version string
}

type serviceInfo struct {
	Name               string `json:"name"`
	Status             string `json:"status"`
	Paused             bool   `json:"paused,omitempty"`
	QueueSite          string `json:"queue_site,omitempty"`
	ScheduleWorkerSite string `json:"schedule_worker_site,omitempty"`
	StripeListenerSite string `json:"stripe_listener_site,omitempty"`
	ReverbSite         string `json:"reverb_site,omitempty"`
	HorizonSite        string `json:"horizon_site,omitempty"`
	WorkerSite         string `json:"worker_site,omitempty"`
}

// isWorker reports whether an /api/services entry is a synthetic per-site
// worker unit rather than a real service preset.
func (s serviceInfo) isWorker() bool {
	return s.QueueSite != "" || s.ScheduleWorkerSite != "" ||
		s.StripeListenerSite != "" || s.ReverbSite != "" ||
		s.HorizonSite != "" || s.WorkerSite != ""
}

// Getter fetches one lerd API path, over HTTP for a tray or in process for lerd-ui.
type Getter func(path string) (io.ReadCloser, error)

// readSnapshot reads the tray's state from lerd's API through get, and the
// toggles through settings, which is only asked once the API has answered.
func readSnapshot(get Getter, settings func() Settings) *Snapshot {
	snap := &Snapshot{}

	type statusResp struct {
		Nginx struct {
			Running bool `json:"running"`
		} `json:"nginx"`
		DNS struct {
			OK      bool   `json:"ok"`
			Status  string `json:"status"`
			Enabled bool   `json:"enabled"`
		} `json:"dns"`
		PHPFPMs []struct {
			Version string `json:"version"`
		} `json:"php_fpms"`
		PHPDefault string `json:"php_default"`
	}
	body, err := get("/api/status")
	if err != nil {
		// API unreachable — return empty (stopped) snapshot. A lerd that is down
		// runs no services, so that emptiness is an answer and the menu may act
		// on it, unlike a services call that simply did not come back.
		snap.ServicesKnown = true
		return snap
	}
	var sr statusResp
	if json.NewDecoder(body).Decode(&sr) == nil {
		snap.Running = sr.Nginx.Running
		snap.NginxRunning = sr.Nginx.Running
		snap.DNSOK = sr.DNS.OK
		snap.DNSDegraded = sr.DNS.Status == "degraded"
		snap.DNSDisabled = !sr.DNS.Enabled
		snap.PHPDefault = sr.PHPDefault
		for _, p := range sr.PHPFPMs {
			snap.PHPVersions = append(snap.PHPVersions, phpInfo{Version: p.Version})
		}
	}
	body.Close()

	s := settings()
	snap.AutostartEnabled = s.Autostart
	snap.LANExposed = s.LANExposed
	snap.LANServicesExposed = s.LANServicesExposed
	snap.DumpsEnabled = s.Dumps
	snap.NotificationsEnabled = s.Notifications
	snap.HighContrastIcon = s.HighContrastIcon
	snap.LatestVersion = s.LatestVersion

	// /api/services — real services only. The per-site worker units mixed into
	// the same payload are counted rather than discarded; the API lists them
	// only while they are up, so the count needs no extra call.
	if body, err := get("/api/services"); err == nil {
		var all []serviceInfo
		if json.NewDecoder(body).Decode(&all) == nil {
			snap.ServicesKnown = true
			for _, svc := range all {
				if svc.isWorker() {
					snap.WorkersRunning++
					continue
				}
				snap.Services = append(snap.Services, svc)
			}
		}
		body.Close()
	}

	// /api/workers/health knows the difference between a worker that is down
	// because its site is paused or idle-suspended and one that actually
	// broke, so the tray reports its verdict instead of counting stopped units.
	if body, err := get("/api/workers/health"); err == nil {
		var health struct {
			Unhealthy []struct {
				Site string `json:"site"`
			} `json:"unhealthy"`
		}
		if json.NewDecoder(body).Decode(&health) == nil {
			for _, u := range health.Unhealthy {
				snap.WorkersDown = append(snap.WorkersDown, u.Site)
			}
		}
		body.Close()
	}

	return snap
}

// State is how a menu entry is doing; each client picks its own colour for it.
type State string

const (
	StateRunning  State = "running"
	StateStopped  State = "stopped"
	StateWarning  State = "warning" // up with a caveat, or stopped on purpose
	StateDisabled State = "disabled"
)

// Menu is the tray menu resolved from lerd's state, for clients that draw it
// themselves: the desktop app, or a status bar module. Labels carry no glyphs.
type Menu struct {
	Running          bool       `json:"running"`
	HighContrastIcon bool       `json:"high_contrast_icon"`
	LatestVersion    string     `json:"latest_version,omitempty"`
	Items            []MenuItem `json:"items"`
}

// MenuItem is one row. Action is the id to post back to run it; a row without
// one is information. Checked is set only on toggles and choices.
type MenuItem struct {
	Label     string     `json:"label,omitempty"`
	Detail    string     `json:"detail,omitempty"`
	Tooltip   string     `json:"tooltip,omitempty"`
	State     State      `json:"state,omitempty"`
	Checked   *bool      `json:"checked,omitempty"`
	Action    string     `json:"action,omitempty"`
	Separator bool       `json:"separator,omitempty"`
	Items     []MenuItem `json:"items,omitempty"`

	// Args is the lerd command behind Action; nil for update, which is not one.
	Args []string `json:"-"`
}

// MenuOptions are the parts of the menu that depend on where lerd runs.
type MenuOptions struct {
	LANToggle bool // false on macOS, where gvproxy always exposes ports 80/443
}

// ActionUpdate opens a terminal running `lerd update` rather than a command.
const ActionUpdate = "update"

func action(label, tooltip string, args ...string) MenuItem {
	return MenuItem{Label: label, Tooltip: tooltip, Action: strings.Join(args, " "), Args: args}
}

func toggle(label, tooltip string, on bool, args ...string) MenuItem {
	it := action(label, tooltip, args...)
	it.Checked = &on
	return it
}

func stateOf(ok bool) State {
	if ok {
		return StateRunning
	}
	return StateStopped
}

// ReadMenu reads lerd's state through get and settings and resolves the menu.
func ReadMenu(get Getter, settings func() Settings, opts MenuOptions) Menu {
	return BuildMenu(readSnapshot(get, settings), opts)
}

// BuildMenu resolves the menu for snap. A client keeps its tray for as long as
// it runs, so the native tray's "Turn off system tray" has no place here.
func BuildMenu(snap *Snapshot, opts MenuOptions) Menu {
	sep := MenuItem{Separator: true}
	m := Menu{Running: snap.Running, HighContrastIcon: snap.HighContrastIcon, LatestVersion: snap.LatestVersion}

	status := MenuItem{Label: "Stopped", State: StateStopped}
	if snap.Running {
		status = MenuItem{Label: "Running", State: StateRunning}
	}
	m.Items = append(m.Items, status, MenuItem{Label: "nginx", State: stateOf(snap.NginxRunning)}, dnsItem(snap))
	if snap.ServicesKnown {
		if w, ok := workersItem(snap); ok {
			m.Items = append(m.Items, w)
		}
	}

	m.Items = append(m.Items, sep, action("Open Dashboard", "Open the Lerd web dashboard", "dashboard"))
	if snap.Running {
		m.Items = append(m.Items, action("Stop Lerd", "Start or stop the Lerd environment", "stop"))
	} else {
		m.Items = append(m.Items, action("Start Lerd", "Start or stop the Lerd environment", "start"))
	}

	m.Items = append(m.Items, sep)
	if svcs := serviceItems(snap); len(svcs) > 0 {
		m.Items = append(m.Items, MenuItem{Label: servicesTitle(snap), Tooltip: "Start or stop a service", Items: svcs})
	}
	if php := phpItems(snap); len(php) > 0 {
		m.Items = append(m.Items, MenuItem{Label: phpTitle(snap), Tooltip: "Switch the default PHP version", Items: php})
	}

	settings := []MenuItem{toggle("Autostart at login", "Toggle lerd autostart on login", snap.AutostartEnabled, "autostart", enableDisable(snap.AutostartEnabled))}
	if opts.LANToggle {
		settings = append(settings, toggle("Expose to LAN", "Toggle whether lerd is reachable from other devices on the local network", snap.LANExposed, "lan", exposeUnexpose(snap.LANExposed)))
	}
	lanSvcs := toggle("Managed service LAN access", "Allow remote access to managed service ports on trusted networks", snap.LANServicesExposed, "lan", "services", offOn(snap.LANServicesExposed))
	if snap.LANServicesExposed && !snap.LANExposed {
		lanSvcs.Detail = "Armed (LAN exposure off)"
	}
	settings = append(settings, lanSvcs,
		toggle("Debug bridge", "Capture dump() / dd() into the lerd dashboard", snap.DumpsEnabled, "dump", offOn(snap.DumpsEnabled)),
		toggle("Notifications", "Globally enable or disable lerd notifications", snap.NotificationsEnabled, "notify", offOn(snap.NotificationsEnabled)),
		toggle("High-contrast icon", "Show an always-visible green running icon instead of the theme-adaptive one", snap.HighContrastIcon, "tray", "icon", iconStyle(snap.HighContrastIcon)),
	)
	m.Items = append(m.Items, sep, MenuItem{Label: "Settings", Tooltip: "Lerd settings", Items: settings})

	update := MenuItem{Label: "Check for update...", Tooltip: "Check for a newer version of Lerd", Action: ActionUpdate}
	if snap.LatestVersion != "" {
		update.Label = "Update to " + snap.LatestVersion
	}
	m.Items = append(m.Items, update, action("Quit Lerd", "Stop all Lerd processes and containers", "quit"))
	return m
}

// Find returns the item the menu offers under the action id. Anything else,
// including a toggle the menu has since flipped, is not found.
func (m Menu) Find(id string) (MenuItem, bool) {
	return findAction(m.Items, id)
}

func findAction(items []MenuItem, id string) (MenuItem, bool) {
	for _, it := range items {
		if id != "" && it.Action == id {
			return it, true
		}
		if sub, ok := findAction(it.Items, id); ok {
			return sub, true
		}
	}
	return MenuItem{}, false
}

func dnsItem(snap *Snapshot) MenuItem {
	switch {
	case snap.DNSDisabled:
		return MenuItem{Label: "dns (disabled)", State: StateDisabled}
	case !snap.DNSOK && snap.DNSDegraded:
		return MenuItem{Label: "dns", State: StateWarning}
	default:
		return MenuItem{Label: "dns", State: stateOf(snap.DNSOK)}
	}
}

// workersItem renders the worker line and reports whether it should show at
// all. A stopped worker is normal on a paused or idle-suspended site, so the
// alarm comes from the health endpoint's verdict, never from a raw count; with
// nothing running and nothing broken the line stays hidden rather than
// standing permanently red.
func workersItem(snap *Snapshot) (MenuItem, bool) {
	switch down := len(snap.WorkersDown); {
	case down == 1:
		return MenuItem{Label: fmt.Sprintf("worker down (%s)", snap.WorkersDown[0]), State: StateStopped}, true
	case down > 1:
		return MenuItem{Label: fmt.Sprintf("%d workers down", down), State: StateStopped}, true
	case snap.WorkersRunning == 1:
		return MenuItem{Label: "1 worker", State: StateRunning}, true
	case snap.WorkersRunning > 1:
		return MenuItem{Label: fmt.Sprintf("%d workers", snap.WorkersRunning), State: StateRunning}, true
	default:
		return MenuItem{}, false
	}
}

// serviceItems renders one item per service. Paused services are a warning:
// they were stopped deliberately, so stopped would read as broken.
func serviceItems(snap *Snapshot) []MenuItem {
	items := make([]MenuItem, 0, len(snap.Services))
	for _, svc := range snap.Services {
		state, verb := StateStopped, "start"
		switch {
		case svc.Paused:
			state = StateWarning
		case svc.Status == "active":
			state, verb = StateRunning, "stop"
		}
		it := action(svc.Name, fmt.Sprintf("Click to %s %s", verb, svc.Name), "service", verb, svc.Name)
		it.State = state
		items = append(items, it)
	}
	return items
}

// phpItems lists the installed PHP versions, checking the current default.
func phpItems(snap *Snapshot) []MenuItem {
	items := make([]MenuItem, 0, len(snap.PHPVersions))
	for _, p := range snap.PHPVersions {
		items = append(items, toggle(p.Version, "Make PHP "+p.Version+" the default", p.Version == snap.PHPDefault, "use", p.Version))
	}
	return items
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

// offOn returns the next-state CLI subcommand for a current boolean: true →
// "off", false → "on".
func offOn(currentlyEnabled bool) string {
	if currentlyEnabled {
		return "off"
	}
	return "on"
}

func enableDisable(enabled bool) string {
	if enabled {
		return "disable"
	}
	return "enable"
}

func exposeUnexpose(exposed bool) string {
	if exposed {
		return "unexpose"
	}
	return "expose"
}

func iconStyle(highContrast bool) string {
	if highContrast {
		return "default"
	}
	return "high-contrast"
}

// ActionMessage is the notification for a finished menu action: what was done,
// or what failed with the last line lerd printed. Opening the dashboard shows
// itself, so only its failure is reported.
func ActionMessage(args []string, err error, stderr string) (title, body string, show bool) {
	doing, done := describeAction(args)
	if err != nil {
		return "Couldn't " + doing, lastLine(stderr, err.Error()), true
	}
	return done, "", done != ""
}

// describeAction names a menu action as a goal ("start redis") and as a result
// ("redis started").
func describeAction(args []string) (doing, done string) {
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	toggle := func(label string, on bool) (string, string) {
		state := "off"
		if on {
			state = "on"
		}
		return "turn " + strings.ToLower(label) + " " + state, label + " " + state
	}
	switch arg(0) {
	case "start":
		return "start Lerd", "Lerd started"
	case "stop", "quit":
		return "stop Lerd", "Lerd stopped"
	case "dashboard":
		return "open the dashboard", ""
	case "service":
		return arg(1) + " " + arg(2), arg(2) + " " + map[string]string{"start": "started", "stop": "stopped"}[arg(1)]
	case "use":
		return "switch to PHP " + arg(1), "PHP " + arg(1) + " is the default"
	case "autostart":
		return toggle("Autostart", arg(1) == "enable")
	case "lan":
		if arg(1) == "services" {
			return toggle("Services on the LAN", arg(2) == "on")
		}
		return toggle("LAN access", arg(1) == "expose")
	case "dump":
		return toggle("Dumps", arg(1) == "on")
	case "notify":
		return toggle("Notifications", arg(1) == "on")
	case "tray":
		return "change the tray icon", "Tray icon: " + arg(2)
	}
	cmd := "lerd " + strings.Join(args, " ")
	return "run " + cmd, cmd + " done"
}

// lastLine is the last non-empty line of out, or fallback when there is none.
func lastLine(out, fallback string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return fallback
}
