//go:build !windows

package tray

import (
	"github.com/geodro/lerd/internal/config"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
	lerdUpdate "github.com/geodro/lerd/internal/update"
	"github.com/geodro/lerd/internal/version"
)

// LocalSettings reads the tray's settings from this machine. The toggles come
// straight from the config, so the tray shows persisted intent.
func LocalSettings() Settings {
	s := Settings{Autostart: lerdSystemd.IsAutostartEnabled()}
	if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
		s.LANExposed = cfg.LAN.Exposed
		s.LANServicesExposed = cfg.LAN.ServicesExposed
		s.Dumps = cfg.IsDumpsEnabled()
		s.Notifications = cfg.IsNotificationsEnabled()
		s.HighContrastIcon = cfg.IsHighContrastTrayIcon()
	}
	// Shared disk cache: no network call while the cache is fresh.
	if info, _ := lerdUpdate.CachedUpdateCheck(version.Version); info != nil {
		s.LatestVersion = info.LatestVersion
	}
	return s
}
