package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// A tunnel started by "lerd share" runs in the user's own terminal, where the
// dashboard's in-process registry cannot see it. Each CLI share drops a small
// file under the run dir for as long as it is up, and the dashboard folds those
// in so a share looks the same wherever it was started from.

// tunnelState is one CLI-owned tunnel, as recorded on disk.
type tunnelState struct {
	Site   string `json:"site"`
	Branch string `json:"branch,omitempty"`
	Tool   string `json:"tool"`
	URL    string `json:"url"`
	PID    int    `json:"pid"`
}

func tunnelStateDir() string {
	return filepath.Join(config.RunDir(), "tunnels")
}

// tunnelStateFile maps a tunnel key to its file. The key is only storage here,
// so anything a path cannot carry becomes an underscore; the entry itself names
// the site and branch it belongs to.
func tunnelStateFile(key string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '.', r == '@':
			return r
		}
		return '_'
	}, key)
	return filepath.Join(tunnelStateDir(), safe+".json")
}

func writeTunnelState(key string, st tunnelState) error {
	defer invalidateTunnelStates()
	if err := os.MkdirAll(tunnelStateDir(), 0755); err != nil {
		return err
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(tunnelStateFile(key), b, 0644)
}

func removeTunnelState(key string) {
	defer invalidateTunnelStates()
	_ = os.Remove(tunnelStateFile(key))
}

// pidAlive reports whether a process is still around. Signal 0 only checks, and
// the CLI share runs as the same user as the daemon reading this.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// A status build asks about every site in turn and the answer is one directory
// either way, so the scan is held briefly. Writes from this process drop it,
// and the directory is part of the key so a change of data dir is a miss.
var (
	statesMu   sync.Mutex
	statesDir  string
	statesAt   time.Time
	statesSeen map[string]tunnelState
)

const tunnelStateTTL = 750 * time.Millisecond

// readTunnelStates returns every live CLI tunnel keyed the same way the
// in-process registry is.
func readTunnelStates() map[string]tunnelState {
	statesMu.Lock()
	defer statesMu.Unlock()
	dir := tunnelStateDir()
	if statesSeen != nil && statesDir == dir && time.Since(statesAt) < tunnelStateTTL {
		return statesSeen
	}
	statesSeen, statesDir, statesAt = scanTunnelStates(), dir, time.Now()
	return statesSeen
}

func invalidateTunnelStates() {
	statesMu.Lock()
	statesSeen = nil
	statesMu.Unlock()
}

// scanTunnelStates reads the run dir, deleting the entries whose process is
// gone. A share killed outright never gets to clean up after itself, so this is
// where those files go.
func scanTunnelStates() map[string]tunnelState {
	live := map[string]tunnelState{}
	entries, err := os.ReadDir(tunnelStateDir())
	if err != nil {
		return live
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(tunnelStateDir(), e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var st tunnelState
		if err := json.Unmarshal(b, &st); err != nil || st.Site == "" || st.URL == "" {
			_ = os.Remove(path)
			continue
		}
		if !pidAlive(st.PID) {
			_ = os.Remove(path)
			continue
		}
		live[tunnelKey(st.Site, st.Branch)] = st
	}
	return live
}

// cliTunnelStatus reports the CLI-started tunnel registered under key.
func cliTunnelStatus(key string) (TunnelInfo, bool) {
	st, ok := readTunnelStates()[key]
	if !ok {
		return TunnelInfo{}, false
	}
	return TunnelInfo{Tool: st.Tool, URL: st.URL, External: true}, true
}

// stopCLITunnel terminates the "lerd share" process behind a CLI tunnel. The
// entry goes now rather than when that share gets around to clearing it, so the
// dashboard never keeps showing a tunnel it just stopped.
func stopCLITunnel(key string) bool {
	st, ok := readTunnelStates()[key]
	if !ok {
		return false
	}
	_ = syscall.Kill(st.PID, syscall.SIGTERM)
	removeTunnelState(key)
	return true
}

// tunnelStateWriter records a CLI share for the dashboard while it runs. It
// doubles as the writer the tool's output is teed through, so a tunnel whose
// URL is only announced in that output gets recorded the moment it appears.
type tunnelStateWriter struct {
	key   string
	mu    sync.Mutex
	state tunnelState
	buf   []byte
	found bool
}

// newTunnelStateWriter starts recording st. A tunnel whose URL is already known,
// as a named Cloudflare tunnel's is, is published straight away.
func newTunnelStateWriter(key string, st tunnelState) *tunnelStateWriter {
	w := &tunnelStateWriter{key: key, state: st}
	if st.URL != "" {
		w.found = true
		_ = writeTunnelState(key, st)
	}
	return w
}

func (w *tunnelStateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.found {
		return len(p), nil
	}
	w.buf = append(w.buf, p...)
	for {
		i := strings.IndexByte(string(w.buf), '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		if u, ok := parseTunnelURL(w.state.Tool, line); ok {
			w.state.URL = u
			w.found = true
			_ = writeTunnelState(w.key, w.state)
			w.buf = nil
			break
		}
	}
	// A tool that never prints a newline must not grow the buffer forever.
	if len(w.buf) > 64*1024 {
		w.buf = w.buf[len(w.buf)-4096:]
	}
	return len(p), nil
}

func (w *tunnelStateWriter) clear() {
	removeTunnelState(w.key)
}
