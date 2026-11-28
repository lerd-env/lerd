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
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/plugins"
)

// installTerminalPlugin installs "shell", whose pty endpoint reports its
// terminal size, echoes each line back and records its pid.
func installTerminalPlugin(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := filepath.Join(plugins.Dir(), "shell")
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "api_version: 1\nname: shell\napi:\n  - {path: term, run: bin/term, pty: true}\n  - {path: hup, run: bin/hup, pty: true}\n" +
		"  - {path: stubborn, run: bin/stubborn, pty: true}\n  - {path: quick, run: bin/quick, pty: true}\n"
	// stubborn's shell exits on a hang-up but leaves a child that ignores one;
	// quick is done before a page can attach.
	stubborn := "#!/bin/sh\ntrap 'exit 0' HUP\nsh -c 'trap \"\" HUP; echo $$ > \"$LERD_PLUGIN_DATA/child\"; while :; do sleep 0.1; done' &\necho ready\nwhile :; do sleep 0.1; done\n"
	for name, body := range map[string]string{"stubborn": stubborn, "quick": "#!/bin/sh\necho all-done\n"} {
		if err := os.WriteFile(filepath.Join(dir, "bin", name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// hup stands in for podman exec, which only ends the shell in the
	// container when it is hung up on, not when it is killed.
	hup := "#!/bin/sh\ntrap 'echo hung-up > \"$LERD_PLUGIN_DATA/hup\"; exit 0' HUP\necho ready\nwhile :; do sleep 0.1; done\n"
	if err := os.WriteFile(filepath.Join(dir, "bin", "hup"), []byte(hup), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho $$ > \"$LERD_PLUGIN_DATA/pid\"\necho \"size $(stty size)\"\n" +
		"while read l; do [ \"$l\" = size ] && echo \"size $(stty size)\" || echo \"got:$l\"; done\n"
	if err := os.WriteFile(filepath.Join(dir, plugins.ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "term"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return plugins.DataDirFor("shell")
}

type termClient struct {
	t       *testing.T
	conn    net.Conn
	br      *bufio.Reader
	seen    strings.Builder
	session string
}

// dialTerminal opens the websocket the dashboard would, with origin as its
// Origin header, and returns the handshake's status with the client.
func dialTerminal(t *testing.T, srv *httptest.Server, path, origin string) (int, *termClient) {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	head := "GET " + path + " HTTP/1.1\r\nHost: 127.0.0.1\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"
	if origin != "" {
		head += "Origin: " + origin + "\r\n"
	}
	_, _ = io.WriteString(conn, head+"\r\n")
	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, &termClient{t: t, conn: conn, br: br}
}

// send writes one masked client frame, as a browser does.
func (c *termClient) send(op byte, payload string) {
	c.t.Helper()
	frame := []byte{0x80 | op, 0x80 | byte(len(payload)), 1, 2, 3, 4}
	for i := range len(payload) {
		frame = append(frame, payload[i]^frame[2+i%4])
	}
	if _, err := c.conn.Write(frame); err != nil {
		c.t.Fatal(err)
	}
}

// waitFor reads binary frames until the terminal has printed want.
func (c *termClient) waitFor(want string) {
	c.t.Helper()
	for !strings.Contains(c.seen.String(), want) {
		hdr := make([]byte, 2)
		if _, err := io.ReadFull(c.br, hdr); err != nil {
			c.t.Fatalf("waiting for %q, read %q: %v", want, c.seen.String(), err)
		}
		n := int(hdr[1] & 0x7f)
		if n == 126 {
			ext := make([]byte, 2)
			_, _ = io.ReadFull(c.br, ext)
			n = int(binary.BigEndian.Uint16(ext))
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(c.br, payload); err != nil {
			c.t.Fatal(err)
		}
		switch op := hdr[0] & 0x0f; op {
		case 0x1:
			var hello struct{ Type, ID string }
			if json.Unmarshal(payload, &hello) == nil && hello.Type == "session" {
				c.session = hello.ID
			}
		case 0x2:
			c.seen.Write(payload)
		default:
			c.t.Fatalf("waiting for %q, got opcode %#x after %q", want, op, c.seen.String())
		}
	}
}

// readUntilClose reads frames until a close frame, reporting whether one came.
func (c *termClient) readUntilClose() bool {
	c.t.Helper()
	for {
		hdr := make([]byte, 2)
		if _, err := io.ReadFull(c.br, hdr); err != nil {
			return false
		}
		n := int(hdr[1] & 0x7f)
		if n == 126 {
			ext := make([]byte, 2)
			_, _ = io.ReadFull(c.br, ext)
			n = int(binary.BigEndian.Uint16(ext))
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(c.br, payload); err != nil {
			return false
		}
		switch hdr[0] & 0x0f {
		case 0x2:
			c.seen.Write(payload)
		case 0x8:
			return true
		}
	}
}

func TestPluginTerminal_RoundTripsKeystrokesAndResizes(t *testing.T) {
	installTerminalPlugin(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	status, c := dialTerminal(t, srv, "/api/plugins/shell/term?cols=100&rows=30", "")
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("handshake answered %d", status)
	}
	c.waitFor("size 30 100")
	c.send(0x2, "hello\r")
	c.waitFor("got:hello")
	c.send(0x1, `{"type":"resize","cols":120,"rows":40}`)
	c.send(0x2, "size\r")
	c.waitFor("size 40 120")
}

// withDashboard counts a dashboard window as open for the test, and ends
// every terminal the test left behind.
func withDashboard(t *testing.T) {
	t.Helper()
	dashboardOpened()
	t.Cleanup(func() {
		dashboardClosed()
		terms.Lock()
		left := make([]*termSession, 0, len(terms.open))
		for _, s := range terms.open {
			left = append(left, s)
		}
		terms.Unlock()
		for _, s := range left {
			s.end()
		}
	})
}

func pidOf(t *testing.T, data string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(data, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	return pid
}

func waitGone(t *testing.T, pid int, why string) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); syscall.Kill(pid, 0) == nil; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("pid %d still running %s", pid, why)
		}
	}
}

func TestPluginTerminal_AnUntouchedTerminalEndsWithItsPage(t *testing.T) {
	data := installTerminalPlugin(t)
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	_, c := dialTerminal(t, srv, "/api/plugins/shell/term", "")
	c.waitFor("size")
	pid := pidOf(t, data)
	c.conn.Close()
	waitGone(t, pid, "after its page left without typing anything")
}

// A terminal the user typed into is kept for the page to come back to: it is
// listed for the site, and going back shows what it printed and carries on.
func TestPluginTerminal_ATouchedTerminalOutlivesItsPage(t *testing.T) {
	data := installTerminalPlugin(t)
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	_, c := dialTerminal(t, srv, "/api/plugins/shell/term", "")
	c.send(0x2, "hello\r")
	c.waitFor("got:hello")
	pid, id := pidOf(t, data), c.session
	if id == "" {
		t.Fatal("the terminal never named its session")
	}
	c.conn.Close()
	time.Sleep(200 * time.Millisecond)
	if syscall.Kill(pid, 0) != nil {
		t.Fatal("a terminal the user typed into ended with its page")
	}

	res, err := http.Get(srv.URL + "/api/plugins/shell/term")
	if err != nil {
		t.Fatal(err)
	}
	var listed []terminalInfo
	_ = json.NewDecoder(res.Body).Decode(&listed)
	res.Body.Close()
	if len(listed) != 1 || listed[0].ID != id {
		t.Fatalf("listed %+v, want the one session %s", listed, id)
	}

	status, back := dialTerminal(t, srv, "/api/plugins/shell/term?session="+id, "")
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("going back answered %d", status)
	}
	back.waitFor("got:hello")
	back.send(0x2, "again\r")
	back.waitFor("got:again")
}

func TestPluginTerminal_EndsWhenTheLastDashboardCloses(t *testing.T) {
	data := installTerminalPlugin(t)
	grace := termGrace.Load()
	termGrace.Store(int64(50 * time.Millisecond))
	t.Cleanup(func() { termGrace.Store(grace) })
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	_, c := dialTerminal(t, srv, "/api/plugins/shell/term", "")
	c.send(0x2, "hi\r")
	c.waitFor("got:hi")
	pid := pidOf(t, data)
	c.conn.Close()

	// A reload drops the dashboard's socket and opens it again within the grace.
	dashboardClosed()
	time.Sleep(20 * time.Millisecond)
	dashboardOpened()
	time.Sleep(150 * time.Millisecond)
	if syscall.Kill(pid, 0) != nil {
		t.Fatal("a reload ended the terminal")
	}
	dashboardClosed()
	waitGone(t, pid, "after the last dashboard window closed")
	dashboardOpened() // balances withDashboard's cleanup
}

func TestPluginTerminal_CloseHangsUpBeforeKilling(t *testing.T) {
	data := installTerminalPlugin(t)
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	_, c := dialTerminal(t, srv, "/api/plugins/shell/hup", "")
	c.waitFor("ready")
	c.send(0x2, "x")
	c.send(0x1, `{"type":"close"}`)

	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if b, _ := os.ReadFile(filepath.Join(data, "hup")); strings.TrimSpace(string(b)) == "hung-up" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("closing never sent SIGHUP, so a shell behind podman exec would outlive the terminal")
		}
	}
}

func TestPluginTerminal_CloseStopsAChildThatIgnoresTheHangUp(t *testing.T) {
	data := installTerminalPlugin(t)
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	_, c := dialTerminal(t, srv, "/api/plugins/shell/stubborn", "")
	c.waitFor("ready")
	var raw []byte
	for deadline := time.Now().Add(3 * time.Second); len(raw) == 0; time.Sleep(20 * time.Millisecond) {
		if raw, _ = os.ReadFile(filepath.Join(data, "child")); time.Now().After(deadline) {
			t.Fatal("the child never started")
		}
	}
	child, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	c.send(0x1, `{"type":"close"}`)
	for deadline := time.Now().Add(5 * time.Second); syscall.Kill(child, 0) == nil; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d that ignores SIGHUP outlived its closed terminal", child)
		}
	}
}

// A run file can be done before the page attaches; the page still sees what
// it printed, and that it has ended, rather than a terminal that never answers.
func TestPluginTerminal_AnEndedTerminalSaysSo(t *testing.T) {
	installTerminalPlugin(t)
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	for range 5 {
		_, c := dialTerminal(t, srv, "/api/plugins/shell/quick", "")
		if !c.readUntilClose() {
			t.Fatalf("no close frame after a terminal that had ended, read %q", c.seen.String())
		}
		if !strings.Contains(c.seen.String(), "all-done") {
			t.Fatalf("its output was lost, read %q", c.seen.String())
		}
	}
}

// Attaching after the session ended, which a quick run file allows, still
// shows its output and then closes, instead of leaving the page waiting.
func TestPluginTerminal_AttachingToAnEndedSessionCloses(t *testing.T) {
	installTerminalPlugin(t)
	p, err := plugins.Find("shell")
	if err != nil {
		t.Fatal(err)
	}
	s, err := startTerminal(p, &p.API[3], map[string]string{}, t.TempDir(), "", "", sizeOf(80, 24))
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(3 * time.Second); findTerminal(s.id, "shell", "quick", "", "") != nil; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the quick terminal never ended")
		}
	}
	server, client := net.Pipe()
	defer client.Close()
	c := &termClient{t: t, conn: client, br: bufio.NewReader(client)}
	go func() {
		if s.attach(&wsConn{conn: server, br: bufio.NewReader(server)}) {
			t.Error("attach reported an ended session as open")
		}
	}()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	if !c.readUntilClose() {
		t.Fatalf("no close frame, read %q", c.seen.String())
	}
	if !strings.Contains(c.seen.String(), "all-done") {
		t.Errorf("the ended session's output was not shown, read %q", c.seen.String())
	}
}

// Every terminal ends with the last dashboard, even one still attached: its
// socket lives in a dashboard window, so it is dead or about to be.
func TestPluginTerminal_AttachedTerminalsEndWithTheLastDashboard(t *testing.T) {
	data := installTerminalPlugin(t)
	grace := termGrace.Load()
	termGrace.Store(int64(50 * time.Millisecond))
	t.Cleanup(func() { termGrace.Store(grace) })
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	_, c := dialTerminal(t, srv, "/api/plugins/shell/term", "")
	c.send(0x2, "hi\r")
	c.waitFor("got:hi")
	pid := pidOf(t, data)
	dashboardClosed()
	waitGone(t, pid, "while attached after the last dashboard closed")
	dashboardOpened()
}

// A page whose network dropped never closes its socket; one that stops
// answering pings is let go like a page that left, so an untouched terminal ends.
func TestPluginTerminal_ASilentPageIsLetGo(t *testing.T) {
	data := installTerminalPlugin(t)
	ping := termPing.Load()
	termPing.Store(int64(50 * time.Millisecond))
	t.Cleanup(func() { termPing.Store(ping) })
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	_, c := dialTerminal(t, srv, "/api/plugins/shell/term", "")
	c.waitFor("size")
	waitGone(t, pidOf(t, data), "after its page stopped answering pings")
}

// A terminal belongs to its plugin: one the user kept ends once the plugin is
// removed, or revoked, rather than running on until the dashboard closes.
func TestPluginTerminal_EndsWhenItsPluginGoes(t *testing.T) {
	data := installTerminalPlugin(t)
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	_, c := dialTerminal(t, srv, "/api/plugins/shell/term", "")
	c.send(0x2, "hi\r")
	c.waitFor("got:hi")
	pid := pidOf(t, data)
	c.conn.Close()
	if err := os.RemoveAll(filepath.Join(plugins.Dir(), "shell")); err != nil {
		t.Fatal(err)
	}
	if left := listTerminals("shell", "term", "", ""); len(left) != 0 {
		t.Errorf("a removed plugin's terminal is still listed: %+v", left)
	}
	waitGone(t, pid, "after its plugin was removed")
}

// An update moves the installed plugin aside under the install lock before
// putting the new one in; a terminal checked in that moment must survive it.
func TestPluginTerminal_SurvivesAnUpdate(t *testing.T) {
	data := installTerminalPlugin(t)
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	_, c := dialTerminal(t, srv, "/api/plugins/shell/term", "")
	c.send(0x2, "hi\r")
	c.waitFor("got:hi")
	pid := pidOf(t, data)

	release, err := git.LockInstall(plugins.Dir(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(plugins.Dir(), "shell")
	if err := os.Rename(dir, dir+".old"); err != nil {
		t.Fatal(err)
	}
	listed := make(chan []terminalInfo)
	go func() { listed <- listTerminals("shell", "term", "", "") }()
	time.Sleep(300 * time.Millisecond)
	if err := os.Rename(dir+".old", dir); err != nil {
		t.Fatal(err)
	}
	release()
	if got := <-listed; len(got) != 1 {
		t.Errorf("listed %+v during an update, want the one terminal", got)
	}
	if syscall.Kill(pid, 0) != nil {
		t.Fatal("an update ended a terminal whose endpoint it kept")
	}
}

func TestPluginTerminal_Refusals(t *testing.T) {
	installTerminalPlugin(t)
	withDashboard(t)
	srv := httptest.NewServer(http.HandlerFunc(handlePluginAPI))
	defer srv.Close()

	if status, _ := dialTerminal(t, srv, "/api/plugins/shell/term", "https://evil.example"); status == http.StatusSwitchingProtocols {
		t.Error("a page on another origin opened a terminal")
	}
	if status, _ := dialTerminal(t, srv, "/api/plugins/shell/term?session=nope", ""); status != http.StatusNotFound {
		t.Errorf("an unknown session answered %d, want 404", status)
	}
	_, c := dialTerminal(t, srv, "/api/plugins/shell/term?site=a", "")
	c.send(0x2, "x\r")
	c.waitFor("got:x")
	if status, _ := dialTerminal(t, srv, "/api/plugins/shell/term?site=b&session="+c.session, ""); status != http.StatusNotFound {
		t.Errorf("another site's page reached the terminal, answered %d", status)
	}
}

// A terminal runs anything as the user, so a remote dashboard reaches it only
// with full access, and a site's page script never does.
func TestPluginTerminal_IsHostOnly(t *testing.T) {
	for _, full := range []bool{false, true} {
		setupConfigDirFullAccess(t, "alice", "s3cret", full)
		installTerminalPlugin(t)
		next := &nextHandler{}
		withRemoteControlGate(next).ServeHTTP(httptest.NewRecorder(), remoteRequest(http.MethodGet, "/api/plugins/shell/term"))
		if next.called != full {
			t.Errorf("full=%v: remote request reached the terminal = %v", full, next.called)
		}
	}
	if !isLoopbackOnlyPluginPath("/api/plugins/shell/term") {
		t.Error("a terminal endpoint should count as loopback only, which also keeps it off page scripts")
	}
}
