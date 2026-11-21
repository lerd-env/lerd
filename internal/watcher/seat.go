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
	procsUp    func() bool   // lerd-ui or lerd-dns running
	otherHolds func() bool   // another user's lerd holds our ports
}

// seatWatchState hands the stack over between users on one machine. What to
// release is read from the units each time, so a lerd start or stop in between
// is always seen; the only thing remembered is whether we owe a start.
type seatWatchState struct {
	prev    systemd.Foreground
	owed    bool // we stopped a stack that was meant to run
	pending bool // ours to start once the ports are free
}

func (s *seatWatchState) decide(enabled bool, fg systemd.Foreground, p seatProbe) seatAction {
	prev := s.prev
	s.prev = fg
	if !enabled {
		s.owed, s.pending = false, false
		return seatNoop
	}
	switch fg {
	case systemd.ForegroundOther:
		s.pending = false
		nginx := p.nginx()
		if nginx != "active" && nginx != "activating" && !p.procsUp() {
			return seatNoop
		}
		// nginx inactive means stopped by hand: release its ports, owe nothing.
		if nginx != "inactive" {
			s.owed = true
		}
		return seatRelease
	case systemd.ForegroundSelf:
		// Logging in starts our units at once, and they fail while the other
		// user's lerd still holds the ports, so that also counts as owed.
		if s.owed || (prev != systemd.ForegroundSelf && blocked(p.nginx())) {
			s.pending = true
		}
		if !s.pending || p.otherHolds() {
			return seatNoop
		}
		s.owed, s.pending = false, false
		return seatResume
	}
	return seatNoop
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
		nginx: func() string { state, _ := podman.UnitStatus("lerd-nginx"); return state },
		procsUp: func() bool {
			ui, _ := podman.UnitStatus("lerd-ui")
			dns, _ := podman.UnitStatus("lerd-dns")
			return ui == "active" || dns == "active"
		},
		otherHolds: func() bool { return cli.OtherUserLerd() != "" },
	}

	switch s.decide(cfg.MultiUser, fg, probe) {
	case seatRelease:
		logger.Info("another user is on screen, releasing lerd's ports")
		releaseSeat()
	case seatResume:
		logger.Info("back on screen, starting lerd")
		if err := cli.RunStart(nil, podman.WatcherUnit); err != nil {
			logger.Warn("starting lerd after a user switch", "err", err)
		}
	}
}

// releaseSeat runs lerd quit minus the watcher, under the lifecycle lock so a
// lerd start in flight cannot bring units back up behind it.
func releaseSeat() {
	unlock, err := podman.LockLifecycle()
	if err != nil {
		logger.Warn("releasing lerd for another user", "err", err)
		return
	}
	defer unlock()
	_ = lifecycle.Quit(lifecycle.SimpleRunner, nil, podman.WatcherUnit)
}
