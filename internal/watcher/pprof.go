package watcher

import (
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/pprofhttp"
)

// PprofSocketPath is where the watcher serves its runtime profiles. A unix
// socket in lerd's own run dir keeps them off the network entirely, so the
// marker file is the only gate it needs; lerd-ui's TCP endpoint adds loopback.
func PprofSocketPath() string {
	return filepath.Join(config.RunDir(), "watcher-pprof.sock")
}

func pprofHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !config.PprofEnabled() {
			http.NotFound(w, r)
			return
		}
		pprofhttp.Serve(w, r)
	})
}

// ServePprof listens on PprofSocketPath until the process exits. Read a
// profile with: curl --unix-socket <path> http://w/debug/pprof/profile?seconds=30
func ServePprof() error {
	path := PprofSocketPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return err
	}
	return http.Serve(ln, pprofHandler())
}
