package ui

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/desktopnotify"
)

// The Lerd desktop app shows lerd's desktop notifications itself while it
// runs. On Windows that keeps them under the app, which can open the dashboard
// at a notification's route in its own window, rather than PowerShell toasts
// raised from inside WSL. It listens on /api/desktop/ws, a socket of its own
// rather than the dashboard's /api/ws: that one counts every connection as an
// open page, which speeds up container polling and rebuilds snapshots for it,
// and the app's listener is no page.
//
// lerd still decides what is raised. dispatchNotification applies the global
// toggle, the kind preferences and the focus rule first, and only what would
// have reached the desktop goes to the app. With no app listening, the native
// emitter takes it as before.

// desktopNotification is one notification as the app receives it.
type desktopNotification struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Body    string `json:"body,omitempty"`
	Route   string `json:"route,omitempty"`
	Urgency string `json:"urgency"`
}

func urgencyName(u desktopnotify.Urgency) string {
	switch u {
	case desktopnotify.UrgencyLow:
		return "low"
	case desktopnotify.UrgencyCritical:
		return "critical"
	default:
		return "normal"
	}
}

// desktopHub fans notifications out to the connected desktop apps.
type desktopHub struct {
	mu    sync.Mutex
	peers map[chan []byte]struct{}
}

var desktopApps = &desktopHub{peers: make(map[chan []byte]struct{})}

func (h *desktopHub) add() chan []byte {
	ch := make(chan []byte, 8)
	h.mu.Lock()
	h.peers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *desktopHub) remove(ch chan []byte) {
	h.mu.Lock()
	delete(h.peers, ch)
	h.mu.Unlock()
}

// listening reports whether a desktop app is connected.
func (h *desktopHub) listening() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.peers) > 0
}

// deliver hands req to every connected app and reports whether one took it.
// An app too far behind to take it in drops it rather than holding lerd up;
// if none took it, the caller raises it natively instead.
func (h *desktopHub) deliver(req desktopnotify.Request) bool {
	frame, err := json.Marshal(desktopNotification{
		Type:    "notification",
		Title:   req.Summary,
		Body:    req.Body,
		Route:   req.Route,
		Urgency: urgencyName(req.Urgency),
	})
	if err != nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	took := false
	for ch := range h.peers {
		select {
		case ch <- frame:
			took = true
		default:
		}
	}
	return took
}

// handleDesktopWS is the desktop app's notification socket. It only writes:
// each frame is one desktopNotification, plus pings so a half-open connection
// is noticed.
func handleDesktopWS(w http.ResponseWriter, r *http.Request) {
	ws, err := wsUpgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// The reader answers pings and close frames on the same connection, and the
	// hand-rolled wsConn is not write-safe.
	var writeMu sync.Mutex
	write := func(f func() error) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return f()
	}

	ch := desktopApps.add()
	defer desktopApps.remove(ch)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if err := ws.SetReadDeadline(time.Now().Add(wsReadTimeout)); err != nil {
				return
			}
			op, payload, err := ws.ReadFrame()
			if err != nil {
				return
			}
			switch op {
			case wsOpPing:
				if write(func() error { return ws.WritePong(payload) }) != nil {
					return
				}
			case wsOpClose:
				_ = write(ws.WriteClose)
				return
			}
		}
	}()
	defer func() {
		ws.Close()
		<-done
	}()

	ping := time.NewTicker(wsPingInterval)
	defer ping.Stop()
	for {
		select {
		case frame := <-ch:
			if write(func() error { return ws.WriteText(frame) }) != nil {
				return
			}
		case <-ping.C:
			if write(func() error { return ws.WritePing(nil) }) != nil {
				return
			}
		case <-done:
			return
		}
	}
}
