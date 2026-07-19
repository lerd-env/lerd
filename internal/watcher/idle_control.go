package watcher

import (
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
)

// startControlSocket binds the idle-suspend control datagram socket and serves
// it until the process exits. Best-effort: a bind failure just means enable/
// disable toggles and activity pings won't reach the engine.
func startControlSocket() {
	if conn, ok := listenDatagram(config.ControlSocketPath()); ok {
		go readDatagrams(conn, func(b []byte) { dispatchControl(string(b)) })
	}
}

// dispatchControl routes one control line: "enable"/"disable" start or tear down
// the idle session (the latter resuming every suspended worker first), and
// "activity <site>" keeps a site awake, ignored entirely while disabled.
func dispatchControl(msg string) {
	kind, arg := parseControlMsg(msg)
	switch kind {
	case "enable":
		enableIdle()
	case "disable":
		disableIdle()
	case "activity":
		if arg != "" && idleActive.Load() {
			activityTracker.TouchSite(arg, time.Now())
			idleEng.OnActivity(arg)
			// Non-blocking coalesced refresh, so a slow/down lerd-ui can't stall
			// the read loop.
			publishSitesChanged()
		}
	case "forget":
		if arg != "" {
			forgetSiteState(arg)
		}
	}
}

// forgetSiteState drops an unlinked site's in-memory request-timing and idle
// state and re-persists both files, so the running watcher stops re-emitting the
// site into the state files after the unlink path has already cleared them.
func forgetSiteState(site string) {
	if reqAggregator != nil {
		reqAggregator.Forget(site)
		_ = reqstats.SaveSnapshot(reqAggregator.Snapshot(), config.RequestStatsFile())
	}
	if activityTracker != nil {
		activityTracker.Forget(site)
		_ = activityTracker.Save(config.IdleActivityFile())
	}
}

// parseControlMsg splits a control datagram into a verb and its optional
// argument ("<verb>" or "<verb> <arg>"), trimming the newline framing. The arg
// is the whole remainder, so a site name containing a space survives intact.
func parseControlMsg(msg string) (kind, arg string) {
	line := strings.TrimSpace(strings.TrimRight(msg, "\n\x00 "))
	if line == "" {
		return "", ""
	}
	if i := strings.IndexByte(line, ' '); i >= 0 {
		return line[:i], strings.TrimSpace(line[i+1:])
	}
	return line, ""
}
