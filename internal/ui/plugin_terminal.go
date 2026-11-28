package ui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/geodro/lerd/internal/plugins"
)

// termScrollback is how much of a terminal's output a page coming back to it
// is shown again.
const termScrollback = 256 << 10

// termGrace is how long terminals outlive the last dashboard, so a reload does
// not end them; termPing is how often an attached page is pinged. Atomic, as
// tests shorten them while an earlier test's terminal may still read them.
var termGrace, termPing atomic.Int64

func init() {
	termGrace.Store(int64(10 * time.Second))
	termPing.Store(int64(25 * time.Second))
}

// termSession is one plugin terminal. It outlives the page that opened it once
// the user has typed into it, until it exits, the page closes it, or the last
// dashboard window goes away.
type termSession struct {
	id, plugin, path, site, branch string
	started                        time.Time
	cmd                            *exec.Cmd
	tty                            *os.File

	mu      sync.Mutex
	conn    *wsConn
	touched bool
	over    bool
	scroll  []byte
	ended   sync.Once
}

var terms = struct {
	sync.Mutex
	open       map[string]*termSession
	dashboards int
	gone       *time.Timer
}{open: map[string]*termSession{}}

// runPluginTerminal serves a pty endpoint: a websocket opens a terminal, or
// with ?session= goes back to one, and a plain GET lists the site's terminals.
// Binary frames carry the bytes each way, text frames resize, close and name it.
func runPluginTerminal(w http.ResponseWriter, r *http.Request, p *plugins.Plugin, e *plugins.Endpoint, env map[string]string, dir string) {
	q := r.URL.Query()
	site, branch := q.Get("site"), q.Get("branch")
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		writeJSON(w, listTerminals(p.Name, e.Path, site, branch))
		return
	}
	var s *termSession
	if id := q.Get("session"); id != "" {
		if s = findTerminal(id, p.Name, e.Path, site, branch); s == nil {
			http.Error(w, "no such terminal: "+id, http.StatusNotFound)
			return
		}
	}
	conn, err := wsUpgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	defer conn.Close()
	if s == nil {
		if s, err = startTerminal(p, e, env, dir, site, branch, termSize(q)); err != nil {
			plugins.Failed(plugins.Failure{Plugin: p.Name, Surface: plugins.SurfaceAPI, What: e.Path, Error: err.Error()})
			_ = conn.WriteBinary([]byte(err.Error() + "\r\n"))
			_ = conn.WriteClose()
			return
		}
	} else {
		_ = pty.Setsize(s.tty, termSize(q))
	}
	if !s.attach(conn) {
		return
	}
	if s.serve(conn) {
		s.end()
		return
	}
	s.detach(conn)
}

func startTerminal(p *plugins.Plugin, e *plugins.Endpoint, env map[string]string, dir, site, branch string, size *pty.Winsize) (*termSession, error) {
	env["TERM"] = "xterm-256color"
	cmd, err := p.Command(e.Run, plugins.SurfaceAPI, nil, env)
	if err != nil {
		return nil, err
	}
	cmd.Dir = dir
	tty, err := pty.StartWithSize(cmd, size)
	if err != nil {
		return nil, err
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	s := &termSession{id: hex.EncodeToString(b), plugin: p.Name, path: e.Path, site: site, branch: branch, started: time.Now(), cmd: cmd, tty: tty}
	terms.Lock()
	terms.open[s.id] = s
	terms.Unlock()
	go s.pump()
	return s, nil
}

// pump keeps the output, sends it to the page attached, and ends the session
// once the process does.
func (s *termSession) pump() {
	buf := make([]byte, 32<<10)
	for {
		n, err := s.tty.Read(buf)
		s.mu.Lock()
		if n > 0 {
			s.scroll = append(s.scroll, buf[:n]...)
			if over := len(s.scroll) - termScrollback; over > 0 {
				s.scroll = slices.Clone(s.scroll[over:])
			}
			if s.conn != nil && s.conn.WriteBinary(buf[:n]) != nil {
				s.conn = nil
			}
		}
		s.mu.Unlock()
		if err != nil {
			s.end()
			return
		}
	}
}

// attach makes conn the page showing the session, replacing any other, and
// shows it what the terminal printed so far. A session that already ended, as
// a quick run file can before the page attaches, shows that and closes.
func (s *termSession) attach(conn *wsConn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.over {
		_ = conn.WriteBinary(s.scroll)
		_ = conn.WriteClose()
		return false
	}
	if s.conn != nil {
		_ = s.conn.WriteClose()
		s.conn.Close()
	}
	s.conn = conn
	hello, _ := json.Marshal(map[string]string{"type": "session", "id": s.id})
	_ = conn.WriteText(hello)
	if len(s.scroll) > 0 {
		_ = conn.WriteBinary(s.scroll)
	}
	return true
}

// serve types conn's input into the terminal until the socket goes away, and
// reports whether the page asked to end it.
func (s *termSession) serve(conn *wsConn) (closed bool) {
	// A page whose network went away never closes its socket, so it is pinged,
	// and one that stops answering is let go like any page that left.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tick := time.NewTicker(time.Duration(termPing.Load()))
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				s.mu.Lock()
				_ = conn.WritePing(nil)
				s.mu.Unlock()
			}
		}
	}()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Duration(termPing.Load())))
		op, payload, err := conn.ReadFrame()
		if err != nil {
			return false
		}
		switch op {
		case wsOpBinary:
			s.mu.Lock()
			s.touched = true
			s.mu.Unlock()
			if _, err := s.tty.Write(payload); err != nil {
				return false
			}
		case wsOpText:
			var msg struct {
				Type       string
				Cols, Rows int
			}
			_ = json.Unmarshal(payload, &msg)
			switch msg.Type {
			case "resize":
				_ = pty.Setsize(s.tty, sizeOf(msg.Cols, msg.Rows))
			case "close":
				return true
			}
		case wsOpPing:
			s.mu.Lock()
			_ = conn.WritePong(payload)
			s.mu.Unlock()
		case wsOpClose:
			return false
		}
	}
}

// detach lets the page go. A terminal nobody typed into is not worth keeping,
// and one nobody can come back to, with no dashboard open, neither.
func (s *termSession) detach(conn *wsConn) {
	s.mu.Lock()
	if s.conn == conn {
		s.conn = nil
	}
	keep := s.touched || s.conn != nil
	s.mu.Unlock()
	// While the last dashboard's grace runs, the timer decides: a reload drops
	// this socket and the dashboard's at the same moment.
	terms.Lock()
	keep = keep && (terms.dashboards > 0 || terms.gone != nil)
	terms.Unlock()
	if !keep {
		s.end()
	}
}

// end stops the terminal once: the page attached learns it closed, and its
// process group is hung up on.
func (s *termSession) end() {
	s.ended.Do(func() {
		terms.Lock()
		delete(terms.open, s.id)
		terms.Unlock()
		s.mu.Lock()
		s.over = true
		if s.conn != nil {
			_ = s.conn.WriteClose()
			s.conn.Close()
			s.conn = nil
		}
		s.mu.Unlock()
		hangUp(s.cmd)
		s.tty.Close()
	})
}

// hangUp ends a terminal's process group the way closing a terminal window
// does, then kills whatever in the group outlived the hang-up. pty.Start made
// the run file a session leader, so its pid names the group it started.
func hangUp(cmd *exec.Cmd) {
	group := -cmd.Process.Pid
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	_ = syscall.Kill(group, syscall.SIGHUP)
	for deadline := time.Now().Add(2 * time.Second); syscall.Kill(group, 0) == nil; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(group, syscall.SIGKILL)
			break
		}
	}
	<-exited
}

type terminalInfo struct {
	ID      string    `json:"id"`
	Started time.Time `json:"started"`
}

// listTerminals answers which terminals a page on this site can go back to,
// oldest first.
func listTerminals(plugin, path, site, branch string) []terminalInfo {
	terms.Lock()
	defer terms.Unlock()
	out := []terminalInfo{}
	for _, s := range terms.open {
		if s.plugin == plugin && s.path == path && s.site == site && s.branch == branch {
			out = append(out, terminalInfo{s.id, s.started})
		}
	}
	slices.SortFunc(out, func(a, b terminalInfo) int { return a.Started.Compare(b.Started) })
	return out
}

func findTerminal(id, plugin, path, site, branch string) *termSession {
	terms.Lock()
	defer terms.Unlock()
	s := terms.open[id]
	if s == nil || s.plugin != plugin || s.path != path || s.site != site || s.branch != branch {
		return nil
	}
	return s
}

// dashboardOpened and dashboardClosed count the dashboard windows open. When
// the last one has been gone for termGrace every terminal ends: one still
// attached lives in a window that is gone, or a socket nobody answers.
func dashboardOpened() {
	terms.Lock()
	defer terms.Unlock()
	terms.dashboards++
	if terms.gone != nil {
		terms.gone.Stop()
		terms.gone = nil
	}
}

func dashboardClosed() {
	terms.Lock()
	defer terms.Unlock()
	if terms.dashboards--; terms.dashboards > 0 {
		return
	}
	terms.gone = time.AfterFunc(time.Duration(termGrace.Load()), endAllTerminals)
}

func endAllTerminals() {
	terms.Lock()
	terms.gone = nil
	if terms.dashboards > 0 {
		terms.Unlock()
		return
	}
	all := make([]*termSession, 0, len(terms.open))
	for _, s := range terms.open {
		all = append(all, s)
	}
	terms.Unlock()
	for _, s := range all {
		s.end()
	}
}

// termSize reads the size the page opened the terminal at, 80x24 when unsaid.
func termSize(q url.Values) *pty.Winsize {
	cols, _ := strconv.Atoi(q.Get("cols"))
	rows, _ := strconv.Atoi(q.Get("rows"))
	return sizeOf(cols, rows)
}

// sizeOf keeps a size within what a terminal can be, so a bad value from the
// page never reaches the kernel.
func sizeOf(cols, rows int) *pty.Winsize {
	clamp := func(v, def int) uint16 {
		if v < 1 || v > 1000 {
			return uint16(def)
		}
		return uint16(v)
	}
	return &pty.Winsize{Cols: clamp(cols, 80), Rows: clamp(rows, 24)}
}
