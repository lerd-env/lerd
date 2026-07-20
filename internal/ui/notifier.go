package ui

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/desktopnotify"
	"github.com/geodro/lerd/internal/push"
)

// notifyAppName is the app_name native notifications are posted under.
const notifyAppName = "Lerd"

type sink int

const (
	sinkOff sink = iota
	sinkBrowser
	sinkNative
)

// notifySink resolves where a notification goes. Native is only chosen when the
// config selects it and a daemon is actually present, so an unsupported host
// transparently falls back to the browser sink instead of dropping alerts. The
// support probe is a function because it costs a D-Bus round trip: an eagerly
// evaluated argument would run it for every delivery, including notifications
// switched off entirely and the browser sink that never touches the bus.
func notifySink(cfg *config.GlobalConfig, nativeSupported func() bool) sink {
	if cfg == nil || !cfg.IsNotificationsEnabled() {
		return sinkOff
	}
	if cfg.NotificationTarget() == config.NotifyTargetNative && nativeSupported() {
		return sinkNative
	}
	return sinkBrowser
}

// nativeRequest maps a notification to a native desktop request, using the
// pre-resolved English Title/Body since the daemon has no page locale.
func nativeRequest(n push.Notification) desktopnotify.Request {
	return desktopnotify.Request{
		AppName: notifyAppName,
		// Icon left empty so the emitter uses the bundled lerd logo.
		Summary: n.Title,
		Body:    n.Body,
		Urgency: desktopnotify.UrgencyFromString(n.Urgency),
		Route:   n.URL,
	}
}

// dispatchNotification is the single choke point for emitting notifications.
// Drops everything when the global notifier toggle is off (lerd notify off /
// tray). The target setting then picks the browser sink (WebSocket + Web Push,
// per-device prefs applying downstream) or the native desktop sink.
func dispatchNotification(n push.Notification) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return
	}
	switch notifySink(cfg, desktopnotify.Supported) {
	case sinkOff:
		return
	case sinkNative:
		// The test notification is a manual action, not a real category, so it
		// always fires; real categories honour the server-side kind prefs.
		if n.Kind != "test" && !cfg.NativeKindEnabled(n.Kind) {
			return
		}
		if _, err := desktopnotify.Emit(nativeRequest(n)); err != nil {
			fmt.Printf("[notifier] native notify failed: %v\n", err)
		}
	default:
		payload, err := n.Payload()
		if err != nil {
			return
		}
		broker.broadcastNotification(payload)
		go func() {
			if err := push.Send(n); err != nil {
				fmt.Printf("[notifier] push send failed: %v\n", err)
			}
		}()
	}
}
