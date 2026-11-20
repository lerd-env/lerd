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

// seatWatchState hands the stack over between users on one machine. It only
// acts on a change of who is on screen, so a stack stopped by hand stays
// stopped, and it starts ours only once the other user's lerd let go of the ports.
type seatWatchState struct {
	prev        systemd.Foreground
	yielded     bool // we released the stack for another user
	pending     bool // ours to start once the ports are free
	handStopped bool // the stack was already stopped by hand when we released it
}

// decide takes nginxBlocked and otherHolds as funcs so the watcher only runs
// them on the ticks that need them.
func (s *seatWatchState) decide(enabled bool, fg systemd.Foreground, nginxBlocked, otherHolds func() bool) seatAction {
	prev := s.prev
	s.prev = fg
	if !enabled {
		s.yielded, s.pending = false, false
		return seatNoop
	}
	switch fg {
	case systemd.ForegroundOther:
		s.pending = false
		if s.yielded {
			return seatNoop
		}
		s.yielded = true
		return seatRelease
	case systemd.ForegroundSelf:
		// Logging in starts our units at once, and they fail while the other
		// user's lerd still holds the ports, so that also counts as ours to start.
		if s.yielded || (prev != systemd.ForegroundSelf && nginxBlocked()) {
			s.yielded, s.pending = false, true
		}
		if !s.pending || otherHolds() {
			return seatNoop
		}
		s.pending = false
		return seatResume
	}
	return seatNoop
}

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
	nginxBlocked := func() bool {
		state, _ := podman.UnitStatus("lerd-nginx")
		return state == "failed" || state == "activating"
	}
	otherHolds := func() bool { return cli.OtherUserLerd() != "" }

	switch s.decide(cfg.MultiUser, fg, nginxBlocked, otherHolds) {
	case seatRelease:
		logger.Info("another user is on screen, releasing lerd's ports")
		s.handStopped = config.IsStopped()
		_ = lifecycle.ShutdownForLogout(lifecycle.SimpleRunner)
	case seatResume:
		logger.Info("back on screen, starting lerd")
		if s.handStopped {
			for _, unit := range lifecycle.QuitProcessUnits(podman.WatcherUnit) {
				_ = podman.StartUnit(unit)
			}
			return
		}
		if err := cli.RunStart(nil, podman.WatcherUnit); err != nil {
			logger.Warn("starting lerd after a user switch", "err", err)
		}
	}
}
