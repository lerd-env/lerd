//go:build !nogui && !windows

package tray

import "github.com/getlantern/systray"

// setItemTitle sets a menu title; the emoji dots render in colour natively.
func setItemTitle(item *systray.MenuItem, title string) { item.SetTitle(title) }

// disableInfoItem greys out a read-only status row.
func disableInfoItem(item *systray.MenuItem) { item.Disable() }
