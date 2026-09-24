package watcher

import "testing"

func TestShareWatchTurnsStreamingOnAndOffAroundAShare(t *testing.T) {
	s := &shareWatchState{}
	if got := s.step(true, false); got != shareTurnOn {
		t.Fatalf("share started: got %v, want on", got)
	}
	if got := s.step(true, true); got != shareNoop {
		t.Fatalf("share continuing: got %v, want noop", got)
	}
	if got := s.step(false, true); got != shareTurnOff {
		t.Fatalf("share ended: got %v, want off", got)
	}
}

func TestShareWatchLeavesAManualOnAlone(t *testing.T) {
	s := &shareWatchState{}
	if got := s.step(true, true); got != shareNoop {
		t.Fatalf("already on by hand: got %v, want noop", got)
	}
	if got := s.step(false, true); got != shareNoop {
		t.Fatalf("share ended, mode was on by hand: got %v, want noop", got)
	}
}

func TestShareWatchRespectsTurningItOffMidShare(t *testing.T) {
	s := &shareWatchState{}
	s.step(true, false)
	if got := s.step(true, false); got != shareNoop {
		t.Fatalf("switched off by hand mid-share: got %v, want noop", got)
	}
	if got := s.step(false, false); got != shareNoop {
		t.Fatalf("share ended after a manual off: got %v, want noop", got)
	}
}

func TestShareWatchCatchesAShareRunningWhenTheFeatureIsEnabled(t *testing.T) {
	s := &shareWatchState{}
	s.seen = true
	if got := s.decide(false, false); got != shareNoop {
		t.Fatalf("disabled during a share: got %v, want noop", got)
	}
	if got := s.decide(true, false); got != shareTurnOn {
		t.Fatalf("enabled mid-share: got %v, want on", got)
	}
	s.seen = false
	if got := s.decide(true, true); got != shareTurnOff {
		t.Fatalf("share ended: got %v, want off", got)
	}
}

func TestShareWatchForgetsWhatItTurnedOnOnceDisabled(t *testing.T) {
	s := &shareWatchState{}
	s.seen = true
	s.decide(true, false)
	s.decide(false, true)
	s.seen = false
	if got := s.decide(true, true); got != shareNoop {
		t.Fatalf("enabled again after the share, mode kept by hand: got %v, want noop", got)
	}
}
