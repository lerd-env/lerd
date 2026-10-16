package ui

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/desktopnotify"
	"github.com/geodro/lerd/internal/push"
)

// listenAsDesktopApp registers a desktop app on the hub for the test's span.
func listenAsDesktopApp(t *testing.T) chan []byte {
	t.Helper()
	ch := desktopApps.add()
	t.Cleanup(func() { desktopApps.remove(ch) })
	return ch
}

func readDesktopFrame(t *testing.T, ch chan []byte) desktopNotification {
	t.Helper()
	select {
	case frame := <-ch:
		var got desktopNotification
		if err := json.Unmarshal(frame, &got); err != nil {
			t.Fatalf("frame %q is not a desktop notification: %v", frame, err)
		}
		return got
	default:
		t.Fatal("the desktop app received nothing")
		return desktopNotification{}
	}
}

// nativeTarget points the test's config at native notifications and replaces
// the emitter, returning what reached it.
func nativeTarget(t *testing.T, supported bool) *[]string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfgDir := config.ConfigDir()
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"),
		[]byte("notifications:\n  target: native\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var emitted []string
	prev, prevSupported := emitDesktopNotification, desktopSupported
	emitDesktopNotification = func(r desktopnotify.Request) (uint32, error) {
		emitted = append(emitted, r.Summary)
		return 0, nil
	}
	desktopSupported = func() bool { return supported }
	t.Cleanup(func() {
		emitDesktopNotification, desktopSupported = prev, prevSupported
	})
	return &emitted
}

func TestDesktopHub_DeliverWithNoAppSaysSo(t *testing.T) {
	if desktopApps.deliver(desktopnotify.Request{Summary: "x"}) {
		t.Error("deliver reported an app took the notification with none connected")
	}
}

func TestDispatchNotification_DesktopAppTakesNativeNotifications(t *testing.T) {
	emitted := nativeTarget(t, true)
	app := listenAsDesktopApp(t)

	dispatchNotification(push.Notification{Kind: "test", Title: "New email", Body: "From acme", URL: "#mail", Urgency: "low"})

	got := readDesktopFrame(t, app)
	want := desktopNotification{Type: "notification", Title: "New email", Body: "From acme", Route: "#mail", Urgency: "low"}
	if got != want {
		t.Errorf("the app got %+v, want %+v", got, want)
	}
	if len(*emitted) != 0 {
		t.Errorf("raised %v natively as well; the app showing it is enough", *emitted)
	}
}

// An app listening stands in for a native emitter, so the native sink is chosen
// even where lerd has none of its own.
func TestDispatchNotification_DesktopAppCountsAsNativeSupport(t *testing.T) {
	nativeTarget(t, false)
	app := listenAsDesktopApp(t)

	dispatchNotification(push.Notification{Kind: "test", Title: "Create finished: myapp"})

	if got := readDesktopFrame(t, app); got.Title != "Create finished: myapp" {
		t.Errorf("the app got %+v", got)
	}
}

// The app only gets what lerd would have raised: the focus rule still holds.
func TestDispatchNotification_DesktopAppKeepsTheFocusRule(t *testing.T) {
	nativeTarget(t, true)
	app := listenAsDesktopApp(t)
	noteFocus(1, true)
	t.Cleanup(func() { dropFocus(1) })

	dispatchNotification(push.Notification{Kind: "mail", Title: "New email"})

	select {
	case frame := <-app:
		t.Errorf("a focused dashboard should hold mail back, but the app got %s", frame)
	default:
	}
}

func TestDispatchNotification_WithoutTheAppRaisesNatively(t *testing.T) {
	emitted := nativeTarget(t, true)

	dispatchNotification(push.Notification{Kind: "test", Title: "Create finished: myapp"})

	if len(*emitted) != 1 {
		t.Errorf("raised %v natively, want the one notification", *emitted)
	}
}

func TestDesktopWSIsLoopbackOnly(t *testing.T) {
	if !isLoopbackOnlyPath("/api/desktop/ws") {
		t.Error("/api/desktop/ws carries every desktop notification, so it must stay with the local user")
	}
}

// TestHandleDesktopWS_DeliversOverTheSocket opens the socket the way the app
// does, with no Origin, and reads a notification off it.
func TestHandleDesktopWS_DeliversOverTheSocket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(handleDesktopWS))
	defer srv.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(conn, "GET /api/desktop/ws HTTP/1.1\r\nHost: 127.0.0.1\r\n"+
		"Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake answered %d", res.StatusCode)
	}

	// The handler registers the app just after the handshake.
	deadline := time.Now().Add(2 * time.Second)
	for !desktopApps.deliver(desktopnotify.Request{Summary: "redis stopped", Urgency: desktopnotify.UrgencyCritical}) {
		if time.Now().After(deadline) {
			t.Fatal("the socket never registered as a desktop app")
		}
		time.Sleep(10 * time.Millisecond)
	}

	head := make([]byte, 2)
	if _, err := io.ReadFull(br, head); err != nil {
		t.Fatal(err)
	}
	if head[0] != 0x81 {
		t.Fatalf("first frame opcode byte %#x, want a final text frame", head[0])
	}
	n := int(head[1] & 0x7f)
	if n == 126 {
		ext := make([]byte, 2)
		if _, err := io.ReadFull(br, ext); err != nil {
			t.Fatal(err)
		}
		n = int(binary.BigEndian.Uint16(ext))
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(br, payload); err != nil {
		t.Fatal(err)
	}
	var got desktopNotification
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "redis stopped" || got.Urgency != "critical" {
		t.Errorf("got %+v", got)
	}
}
