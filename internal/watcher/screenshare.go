package watcher

import (
	"time"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/screenshare"
)

type shareAction int

const (
	shareNoop shareAction = iota
	shareTurnOn
	shareTurnOff
)

// shareWatchState acts only on a share starting or stopping, so a manual
// toggle in between always wins, and it only undoes a streaming mode it turned
// on itself.
type shareWatchState struct {
	sharing bool
	autoOn  bool
}

func (s *shareWatchState) step(sharing, streaming bool) shareAction {
	if sharing == s.sharing {
		if sharing && s.autoOn && !streaming {
			s.autoOn = false // switched off by hand mid-share
		}
		return shareNoop
	}
	s.sharing = sharing
	if sharing && !streaming {
		s.autoOn = true
		return shareTurnOn
	}
	if !sharing && s.autoOn {
		s.autoOn = false
		return shareTurnOff
	}
	return shareNoop
}

// WatchScreenShare turns streaming mode on the moment a screen share starts,
// when the user opted in with `lerd streaming auto on`. It follows PipeWire's
// event stream rather than polling, since every second of lag is a second the
// private sites are on someone else's screen. Hosts without PipeWire return
// immediately; a PipeWire restart is followed after retryDelay.
func WatchScreenShare(retryDelay time.Duration) {
	if !screenshare.Supported() {
		return
	}
	state := &shareWatchState{}
	for {
		err := screenshare.Follow(func(sharing bool) { onShareChange(state, sharing) })
		if err != nil {
			logger.Warn("screen share detection stopped", "err", err)
		}
		time.Sleep(retryDelay)
	}
}

func onShareChange(state *shareWatchState, sharing bool) {
	cfg, err := config.LoadGlobal()
	if err != nil || !cfg.UI.StreamingAuto {
		return
	}
	switch state.step(sharing, cfg.UI.StreamingMode) {
	case shareTurnOn:
		logger.Info("screen share started, streaming mode on")
		err = cli.SetStreamingMode(true)
	case shareTurnOff:
		logger.Info("screen share ended, streaming mode off")
		err = cli.SetStreamingMode(false)
	}
	if err != nil {
		logger.Warn("switching streaming mode failed", "err", err)
	}
}
