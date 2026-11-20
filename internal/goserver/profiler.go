package goserver

import (
	"log"
	"net/http"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/fcgi"
)

const profilerHost = "profiler.localhost"

// profilerStatePath answers whether profiling is on, which the profiler page
// reads to know what the server is actually doing.
const profilerStatePath = "/_lerd/profiler-state"

// serveProfiler serves the SPX dashboard as the profiler vhost does: SPX
// answers its UI before the bridge script runs, so the script only has to
// exist, and the key is what lets SPX answer at all.
func (h *Handler) serveProfiler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == profilerStatePath {
		skipAccessFeed(w)
		w.Header().Set("Content-Type", "text/plain")
		state := "off"
		if cfg, err := config.LoadGlobal(); err == nil && cfg.IsProfilerEnabled() {
			state = "on"
		}
		w.Write([]byte(state))
		return
	}
	if h.ProfilerUpstream == nil {
		http.NotFound(w, r)
		return
	}
	addr, script := h.ProfilerUpstream()
	key, _ := config.LoadOrGenerateProfilerKey()
	params := map[string]string{"SCRIPT_FILENAME": script, "HTTP_COOKIE": "SPX_KEY=" + key}
	if err := fcgi.Serve(w, r, addr, params, 0); err != nil {
		log.Printf("lerd web server: profiler: %v", err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
	}
}
