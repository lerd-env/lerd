//go:build linux

package desktopnotify

import (
	"os/exec"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	notifyDest = "org.freedesktop.Notifications"
	notifyPath = "/org/freedesktop/Notifications"
)

var (
	routeMu      sync.Mutex
	routes       = map[uint32]string{}
	listenerOnce sync.Once
)

// Supported reports whether native notifications can be delivered on this host.
// True when a daemon already owns the well-known name (GNOME, KDE, which run it
// as part of the session) or when the name is D-Bus activatable (mako, dunst,
// xfce4-notifyd, which auto-start on the first Notify). False on a headless host
// with no session bus and no daemon, so callers fall back to the browser sink.
func Supported() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	bus := conn.BusObject()

	var has bool
	if err := bus.Call("org.freedesktop.DBus.NameHasOwner", 0, notifyDest).Store(&has); err == nil && has {
		return true
	}

	var activatable []string
	if err := bus.Call("org.freedesktop.DBus.ListActivatableNames", 0).Store(&activatable); err == nil {
		for _, name := range activatable {
			if name == notifyDest {
				return true
			}
		}
	}
	return false
}

// Emit shows a native notification and returns the daemon-assigned id.
func Emit(req Request) (uint32, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return 0, err
	}
	obj := conn.Object(notifyDest, notifyPath)
	hints := map[string]dbus.Variant{
		"urgency": dbus.MakeVariant(byte(req.Urgency)),
	}
	icon := req.Icon
	if icon == "" {
		icon = IconPath()
	}
	// A route makes the popup clickable: "default" is the whole-body click most
	// daemons fire, and a visible "Open" action for those that render buttons.
	var actions []string
	if req.Route != "" {
		actions = []string{"default", "", "open", "Open Lerd"}
	}
	call := obj.Call(notifyDest+".Notify", 0,
		req.AppName, // app_name
		uint32(0),   // replaces_id
		icon,        // app_icon
		req.Summary, // summary
		req.Body,    // body
		actions,     // actions
		hints,       // hints
		int32(-1),   // expire_timeout, -1 = daemon default
	)
	if call.Err != nil {
		return 0, call.Err
	}
	var id uint32
	if err := call.Store(&id); err != nil {
		return 0, err
	}
	if req.Route != "" {
		trackRoute(id, req.Route)
	}
	return id, nil
}

// trackRoute records the route to open for a notification id and starts the
// ActionInvoked listener on first use.
func trackRoute(id uint32, route string) {
	routeMu.Lock()
	routes[id] = route
	routeMu.Unlock()
	listenerOnce.Do(startActionListener)
}

// startActionListener subscribes to the notification daemon's signals and opens
// the tracked route when the user clicks a lerd notification.
func startActionListener() {
	conn, err := dbus.SessionBus()
	if err != nil {
		return
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(notifyPath),
		dbus.WithMatchInterface(notifyDest),
	); err != nil {
		return
	}
	ch := make(chan *dbus.Signal, 16)
	conn.Signal(ch)
	go func() {
		for sig := range ch {
			if len(sig.Body) == 0 {
				continue
			}
			id, ok := sig.Body[0].(uint32)
			if !ok {
				continue
			}
			switch sig.Name {
			case notifyDest + ".ActionInvoked":
				routeMu.Lock()
				route, tracked := routes[id]
				delete(routes, id)
				routeMu.Unlock()
				if tracked {
					go openRoute(route)
				}
			case notifyDest + ".NotificationClosed":
				routeMu.Lock()
				delete(routes, id)
				routeMu.Unlock()
			}
		}
	}()
}

// openRoute focuses the desktop app via its lerd:// scheme when it is the
// registered handler, otherwise opens the dashboard route in the browser.
func openRoute(route string) {
	if out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/lerd").Output(); err == nil && len(out) > 0 {
		_ = exec.Command("xdg-open", appSchemeURL(route)).Run()
		return
	}
	_ = exec.Command("xdg-open", dashboardURL(route)).Run()
}
