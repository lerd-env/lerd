//go:build linux

package desktopnotify

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	notifyDest = "org.freedesktop.Notifications"
	notifyPath = "/org/freedesktop/Notifications"
	// dbusTimeout bounds every bus call. Callers are synchronous request
	// handlers in a long-lived daemon, so a wedged notification daemon would
	// otherwise pin one goroutine per event with no bound on how many pile up.
	dbusTimeout = 3 * time.Second
	// maxTrackedRoutes caps the click-route map. A daemon that shows popups
	// without ever emitting ActionInvoked or NotificationClosed would grow it
	// without limit over the process lifetime.
	maxTrackedRoutes = 256
)

var (
	busMu   sync.Mutex
	busConn *dbus.Conn

	routeMu        sync.Mutex
	routes         = map[uint32]string{}
	routeOrder     []uint32
	listenerActive bool
)

// sessionBus returns a cached connection to the session bus. Autostart is
// deliberately off: godbus falls back to a bare dbus-launch, which spawns a
// dbus-daemon that outlives lerd, so `lerd start` over SSH would leave an
// orphan behind on every host that has dbus installed but no session.
func sessionBus() (*dbus.Conn, error) {
	busMu.Lock()
	defer busMu.Unlock()
	if busConn != nil && busConn.Connected() {
		return busConn, nil
	}
	conn, err := dbus.SessionBusPrivateNoAutoStartup()
	if err != nil {
		return nil, err
	}
	if err := conn.Auth(nil); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.Hello(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	busConn = conn
	return conn, nil
}

// call issues a bus call that gives up after dbusTimeout instead of blocking
// on context.Background() the way a zero-flag Call does.
func call(obj dbus.BusObject, method string, args ...any) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), dbusTimeout)
	defer cancel()
	return obj.CallWithContext(ctx, method, 0, args...)
}

// Supported reports whether native notifications can be delivered on this host.
// True when a daemon already owns the well-known name (GNOME, KDE, which run it
// as part of the session) or when the name is D-Bus activatable (mako, dunst,
// xfce4-notifyd, which auto-start on the first Notify). False on a headless host
// with no session bus and no daemon, so callers fall back to the browser sink.
func Supported() bool {
	conn, err := sessionBus()
	if err != nil {
		return false
	}
	bus := conn.BusObject()

	var has bool
	if err := call(bus, "org.freedesktop.DBus.NameHasOwner", notifyDest).Store(&has); err == nil && has {
		return true
	}

	var activatable []string
	if err := call(bus, "org.freedesktop.DBus.ListActivatableNames").Store(&activatable); err == nil {
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
	conn, err := sessionBus()
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
	c := call(obj, notifyDest+".Notify",
		req.AppName, // app_name
		uint32(0),   // replaces_id
		icon,        // app_icon
		req.Summary, // summary
		req.Body,    // body
		actions,     // actions
		hints,       // hints
		int32(-1),   // expire_timeout, -1 = daemon default
	)
	if c.Err != nil {
		return 0, c.Err
	}
	var id uint32
	if err := c.Store(&id); err != nil {
		return 0, err
	}
	if req.Route != "" {
		trackRoute(id, req.Route)
	}
	return id, nil
}

// trackRoute records the route to open for a notification id, starting the
// ActionInvoked listener on first use. A failed start is not latched, so a bus
// that was briefly unavailable doesn't leave clicks dead for the rest of the
// process; without a listener the route is dropped rather than tracked forever.
func trackRoute(id uint32, route string) {
	routeMu.Lock()
	defer routeMu.Unlock()
	if !listenerActive {
		if !startActionListener() {
			return
		}
		listenerActive = true
	}
	routes[id] = route
	routeOrder = append(routeOrder, id)
	for len(routeOrder) > maxTrackedRoutes {
		delete(routes, routeOrder[0])
		routeOrder = routeOrder[1:]
	}
}

// startActionListener subscribes to the notification daemon's signals and opens
// the tracked route when the user clicks a lerd notification. Reports whether
// the subscription is live. Swappable for tests.
var startActionListener = func() bool {
	conn, err := sessionBus()
	if err != nil {
		return false
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(notifyPath),
		dbus.WithMatchInterface(notifyDest),
	); err != nil {
		return false
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
	return true
}

// openRoute sends the click to the best available target: the desktop app via
// its lerd:// scheme, then an installed PWA via web+lerd://, then the dashboard
// in the browser.
func openRoute(route string) {
	switch {
	case hasSchemeHandler("lerd"):
		_ = exec.Command("xdg-open", appSchemeURL(route)).Run()
	case hasSchemeHandler("web+lerd"):
		_ = exec.Command("xdg-open", pwaSchemeURL(route)).Run()
	default:
		_ = exec.Command("xdg-open", browserURL(route)).Run()
	}
}

// hasSchemeHandler reports whether a default application is registered for the
// given URL scheme.
func hasSchemeHandler(scheme string) bool {
	out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/"+scheme).Output()
	return err == nil && len(bytes.TrimSpace(out)) > 0
}

// AppInstalled reports whether the Lerd desktop app is registered as the lerd://
// scheme handler, so callers can prefer it over a browser.
func AppInstalled() bool {
	return hasSchemeHandler("lerd")
}

// OpenApp focuses (or launches) the desktop app at the given dashboard route via
// its lerd:// scheme. Start, not Run: xdg-open can outlive the handoff, which
// would block `lerd dashboard` and freeze the tray's Dashboard item behind it.
func OpenApp(route string) error {
	return exec.Command("xdg-open", appSchemeURL(route)).Start()
}
