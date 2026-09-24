package watcher

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
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
	mu      sync.Mutex
	seen    bool // what PipeWire last reported
	sharing bool // the share as acted on, only tracked while enabled
	autoOn  bool
}

// decide applies the feature opt-in to the last seen share. While disabled it
// forgets the share, so enabling it mid-share counts as the share starting.
func (s *shareWatchState) decide(enabled, streaming bool) shareAction {
	if !enabled {
		s.sharing, s.autoOn = false, false
		return shareNoop
	}
	return s.step(s.seen, streaming)
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
// when the user enabled the feature. It follows PipeWire's
// event stream rather than polling, since every second of lag is a second the
// private sites are on someone else's screen. Hosts without PipeWire return
// immediately; a PipeWire restart is followed after retryDelay. PipeWire is
// silent while a share runs, so config.yaml is watched too, catching the
// feature being enabled mid-share and a manual toggle.
func WatchScreenShare(retryDelay time.Duration) {
	if !screenshare.Supported() {
		return
	}
	state := &shareWatchState{}
	go watchConfigForShare(state)
	for {
		err := screenshare.Follow(func(sharing bool) {
			state.mu.Lock()
			state.seen = sharing
			state.mu.Unlock()
			applyShareState(state)
		})
		if err != nil {
			logger.Warn("screen share detection stopped", "err", err)
		}
		time.Sleep(retryDelay)
	}
}

// watchConfigForShare re-applies the share state on every config.yaml write.
// The directory is watched because saves replace the file.
func watchConfigForShare(state *shareWatchState) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Warn("screen share config watch failed", "err", err)
		return
	}
	defer w.Close()
	file := config.GlobalConfigFile()
	if err := w.Add(filepath.Dir(file)); err != nil {
		logger.Warn("screen share config watch failed", "err", err)
		return
	}
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Name == file && ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
				applyShareState(state)
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			logger.Warn("screen share config watch", "err", err)
		}
	}
}

func applyShareState(state *shareWatchState) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return
	}
	state.mu.Lock()
	action := state.decide(cfg.UI.StreamingEnabled, cfg.UI.StreamingMode)
	state.mu.Unlock()
	switch action {
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
