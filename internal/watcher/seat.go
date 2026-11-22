package watcher

import (
	"time"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/lifecycle"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/systemd"
)

type seatAction int

const (
	seatNoop seatAction = iota
	seatRelease
	seatResume
)

// seatProbe reads the stack as it is now. Funcs, so the watcher only asks
// systemd and ss on the ticks that need them.
type seatProbe struct {
	nginx      func() string // lerd-nginx's ActiveState
	owed       func() bool   // a release stopped a running stack (config.SeatReleased)
	otherHolds func() bool   // another user's lerd holds our ports
}

// seatWatchState hands the stack over between users on one machine. Every
// switch away releases the whole stack; whether a start is owed lives on disk,
// so a restarted watcher still knows and a lerd stop by hand cancels it.
type seatWatchState struct {
	prev    systemd.Foreground
	pending bool // ours to start once the ports are free
}

func (s *seatWatchState) decide(enabled bool, fg systemd.Foreground, p seatProbe) seatAction {
	prev := s.prev
	s.prev = fg
	if !enabled {
		s.pending = false
		return seatNoop
	}
	switch fg {
	case systemd.ForegroundOther:
		s.pending = false
		if prev == systemd.ForegroundOther {
			return seatNoop
		}
		return seatRelease
	case systemd.ForegroundSelf:
		// Logging in starts our units at once, and they fail while the other
		// user's lerd still holds the ports, so that also counts as owed.
		if p.owed() || (prev != systemd.ForegroundSelf && blocked(p.nginx())) {
			s.pending = true
		}
		if !s.pending || p.otherHolds() {
			return seatNoop
		}
		s.pending = false
		return seatResume
	}
	return seatNoop
}

// seatHeldByOther reports another user's desktop on screen in multi-user mode,
// when nothing of ours may take a port, not even a brief start for a snapshot.
var seatHeldByOther = func() bool {
	cfg, err := config.LoadGlobal()
	return err == nil && cfg.MultiUser && systemd.SeatForeground() == systemd.ForegroundOther
}

func blocked(nginx string) bool { return nginx == "failed" || nginx == "activating" }

// WatchSeat follows who is on screen when multi-user mode is on: it releases
// lerd's ports when another user's desktop takes the screen and starts the
// stack again when ours comes back.
func WatchSeat(interval time.Duration) {
	s := &seatWatchState{}
	for range time.Tick(interval) {
		applySeat(s)
	}
}

func applySeat(s *seatWatchState) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return
	}
	fg := systemd.ForegroundUnknown
	if cfg.MultiUser {
		fg = systemd.SeatForeground()
	}
	probe := seatProbe{
		nginx:      func() string { state, _ := podman.UnitStatus("lerd-nginx"); return state },
		owed:       config.SeatReleased,
		otherHolds: func() bool { return cli.OtherUserLerd() != "" },
	}

	switch s.decide(cfg.MultiUser, fg, probe) {
	case seatRelease:
		logger.Info("another user is on screen, releasing lerd's ports")
		releaseSeat()
	case seatResume:
		// lerd start clears the marker, unless it refuses because the screen
		// changed hands again, which leaves the start owed for next time.
		logger.Info("back on screen, starting lerd")
		if err := cli.RunStart(nil, podman.WatcherUnit); err != nil {
			logger.Warn("starting lerd after a user switch", "err", err)
		}
	}
}

// releaseSeat runs lerd quit minus the watcher, and owes a start only if the
// stack was meant to run. Both under the lifecycle lock, so a lerd start or
// stop in flight finishes first and its stop marker is the one we read.
func releaseSeat() {
	unlock, err := podman.LockLifecycle()
	if err != nil {
		logger.Warn("releasing lerd for another user", "err", err)
		return
	}
	defer unlock()
	// A brief snapshot start holds svcMu; waiting for it means the database it
	// started is stopped again before the other user's stack needs the port.
	if idleEng != nil {
		idleEng.svcMu.Lock()
		defer idleEng.svcMu.Unlock()
	}
	if !config.IsStopped() {
		_ = config.MarkSeatReleased()
	}
	_ = lifecycle.Quit(lifecycle.SimpleRunner, nil, podman.WatcherUnit)
}
