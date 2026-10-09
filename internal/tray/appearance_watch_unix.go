//go:build !nogui && !windows

package tray

import (
	"context"

	"github.com/godbus/dbus/v5"
)

// watchAppearance reports whether the desktop panel is light, once on startup
// and again every time the user flips their light/dark preference. It is a
// no-op on systems without an XDG desktop portal (older or headless setups),
// which leaves the existing white running icon in place.
func watchAppearance(ctx context.Context, onChange func(light bool)) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	if scheme, ok := readColorScheme(conn); ok {
		onChange(schemeIsLight(scheme))
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(portalPath),
		dbus.WithMatchInterface(settingsIface),
		dbus.WithMatchMember("SettingChanged"),
	); err != nil {
		conn.Close()
		return
	}
	sigCh := make(chan *dbus.Signal, 8)
	conn.Signal(sigCh)
	go func() {
		defer conn.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-sigCh:
				if !ok {
					return
				}
				if light, ok := signalColorScheme(sig); ok {
					onChange(light)
				}
			}
		}
	}()
}
