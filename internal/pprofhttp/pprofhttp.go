// Package pprofhttp routes /debug/pprof/ requests to the runtime profiles, for
// the lerd daemons that expose them behind their own access gates.
package pprofhttp

import (
	"net/http"
	"net/http/pprof"
	"strings"
)

// Serve answers one /debug/pprof/ request. Callers gate access first.
func Serve(w http.ResponseWriter, r *http.Request) {
	switch strings.TrimPrefix(r.URL.Path, "/debug/pprof/") {
	case "cmdline":
		pprof.Cmdline(w, r)
	case "profile":
		pprof.Profile(w, r)
	case "symbol":
		pprof.Symbol(w, r)
	case "trace":
		pprof.Trace(w, r)
	default:
		// Index also serves the named runtime profiles (heap, goroutine,
		// allocs, block, mutex) from its own path parsing.
		pprof.Index(w, r)
	}
}
