//go:build !nogui && windows

package tray

import (
	"context"
	"time"

	"golang.org/x/sys/windows/registry"
)

const (
	personalizeKey   = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	appearancePollMs = 5000
)

// taskbarIsLight reads the taskbar theme, which is what the tray icon sits on.
// A missing value (older Windows) keeps the white icon.
func taskbarIsLight() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, personalizeKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("SystemUsesLightTheme")
	return err == nil && v == 1
}

// watchAppearance reports the taskbar light/dark state on startup and whenever
// it changes. Windows has no change signal this process can subscribe to
// cheaply, so it polls the registry.
func watchAppearance(ctx context.Context, onChange func(light bool)) {
	last := taskbarIsLight()
	onChange(last)
	go func() {
		t := time.NewTicker(appearancePollMs * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if now := taskbarIsLight(); now != last {
					last = now
					onChange(now)
				}
			}
		}
	}()
}
