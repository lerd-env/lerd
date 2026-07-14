package siteinfo

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// healthDialTimeout bounds one reachability probe so a hung dev server can't
// stall the snapshot. Short because the server is on the same host.
const healthDialTimeout = 300 * time.Millisecond

// dialProbe opens and closes a TCP connection; a var so tests probe without a
// real socket.
var dialProbe = func(address string) error {
	c, err := net.DialTimeout("tcp", address, healthDialTimeout)
	if err != nil {
		return err
	}
	return c.Close()
}

// WorkerServerReachable reports whether a worker's declared server is accepting
// connections. probed is false when the worker declares no usable health source
// (no block, a URL file that isn't there yet, or a file with no host:port), in
// which case the caller keeps the process-only liveness check. A missing URL
// file is treated as not-probeable rather than a failure: idle-suspend clears
// vite's public/hot while the unit is briefly still up, and the real failure the
// block catches is a stale file whose advertised port is refused. When probed is
// true, reachable is the result of a short TCP dial.
// activeEnter (when non-zero) suppresses a false positive: a url_file older than
// the activation is a stale leftover, so it is not dialed. Plenty of healthy
// setups never write the file at all (a custom vite hotFile, vite build --watch,
// a project that cleans public/), so its absence is never itself a failure.
func WorkerServerReachable(sitePath string, h *config.WorkerHealth, activeEnter time.Time) (reachable, probed bool) {
	if h == nil || h.URLFile == "" {
		return false, false
	}
	path := filepath.Join(sitePath, h.URLFile)
	info, statErr := os.Stat(path)
	if statErr != nil {
		return false, false
	}
	// A url_file older than this activation is a stale leftover from before a
	// restart; its advertised port may have moved, so don't dial it.
	if !activeEnter.IsZero() && info.ModTime().Before(activeEnter) {
		return false, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	addr := hostPortFromURL(strings.TrimSpace(string(data)))
	if addr == "" {
		return false, false
	}
	if err := dialProbe(addr); err != nil {
		return false, true
	}
	return true, true
}

// hostPortFromURL pulls a dialable host:port out of a server URL like
// "http://[::1]:5173" or "http://localhost:5173". Returns "" when there is no
// port, since then there is nothing to probe. A host-less URL defaults to
// localhost, which is where a same-host dev server binds.
func hostPortFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Port() == "" {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		host = "localhost"
	}
	return net.JoinHostPort(host, u.Port())
}
