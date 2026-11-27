package ui

import (
	"net/http"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/pprofhttp"
)

// handlePprof serves the Go runtime profiling endpoints under /debug/pprof/,
// behind two independent gates.
//
// The first is the marker file (config.PprofEnabled), so profiling is off
// until someone deliberately turns it on. It is checked per request rather
// than at startup because the daemon worth profiling is usually one that is
// misbehaving right now, and a restart to enable it would discard exactly the
// state being chased.
//
// The second is loopback. lerd-ui binds 0.0.0.0 so LAN clients can reach it
// when lan:expose is on, and these endpoints expose goroutine stacks, the
// process command line, and heap contents; profile and trace also let a caller
// pin a core for as long as they ask. Neither belongs off-host, so the same
// boundary the remote-control middleware uses applies here too.
//
// A blocked request gets 404 rather than 403: a daemon with profiling off
// should look like it has no profiling surface at all.
func handlePprof(w http.ResponseWriter, r *http.Request) {
	if !config.PprofEnabled() || !isLoopbackRequest(r) {
		http.NotFound(w, r)
		return
	}
	pprofhttp.Serve(w, r)
}
