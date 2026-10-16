package tray

// Settings are the persisted lerd toggles the tray menu shows.
type Settings struct {
	Autostart          bool
	LANExposed         bool
	LANServicesExposed bool
	Dumps              bool
	Notifications      bool
	HighContrastIcon   bool
	LatestVersion      string // e.g. "v1.2.0" when an update is available
}
