package watcher

import (
	"testing"

	"github.com/geodro/lerd/internal/systemd"
)

func TestSeatWatchState(t *testing.T) {
	const (
		on  = "active"
		off = "inactive"
		blk = "activating" // a unit retrying against ports another user holds
	)
	type tick struct {
		enabled    bool
		fg         systemd.Foreground
		nginx      string
		owed       bool // the seat-released marker is on disk
		otherHolds bool
		want       seatAction
	}
	cases := []struct {
		name  string
		ticks []tick
	}{
		{"switching away releases once, switching back resumes what is owed", []tick{
			{true, systemd.ForegroundSelf, on, false, false, seatNoop},
			{true, systemd.ForegroundOther, on, false, false, seatRelease},
			{true, systemd.ForegroundOther, off, true, false, seatNoop},
			{true, systemd.ForegroundSelf, off, true, false, seatResume},
			{true, systemd.ForegroundSelf, on, false, false, seatNoop},
		}},
		{"resume waits until the other user's lerd lets go of the ports", []tick{
			{true, systemd.ForegroundOther, on, false, false, seatRelease},
			{true, systemd.ForegroundSelf, off, true, true, seatNoop},
			{true, systemd.ForegroundSelf, off, true, false, seatResume},
		}},
		{"every switch away releases, even with the stack already down", []tick{
			{true, systemd.ForegroundOther, on, false, false, seatRelease},
			{true, systemd.ForegroundSelf, off, true, true, seatNoop},
			{true, systemd.ForegroundOther, off, true, false, seatRelease},
			{true, systemd.ForegroundSelf, off, true, false, seatResume},
		}},
		{"nothing owed, as after a stop by hand, starts nothing", []tick{
			{true, systemd.ForegroundOther, off, false, false, seatRelease},
			{true, systemd.ForegroundSelf, off, false, false, seatNoop},
			{true, systemd.ForegroundSelf, off, false, false, seatNoop},
		}},
		{"a restarted watcher still resumes what the marker owes", []tick{
			{true, systemd.ForegroundOther, off, true, false, seatRelease},
			{true, systemd.ForegroundSelf, off, true, false, seatResume},
		}},
		{"logging in after the other user's lerd blocked our start", []tick{
			{true, systemd.ForegroundSelf, blk, false, true, seatNoop},
			{true, systemd.ForegroundSelf, blk, false, false, seatResume},
			{true, systemd.ForegroundSelf, blk, false, false, seatNoop},
		}},
		{"the login screen in between still counts as a switch away", []tick{
			{true, systemd.ForegroundSelf, on, false, false, seatNoop},
			{true, systemd.ForegroundNobody, on, false, false, seatNoop},
			{true, systemd.ForegroundOther, on, false, false, seatRelease},
		}},
		{"the login screen and an unknown seat leave the stack alone", []tick{
			{true, systemd.ForegroundNobody, on, true, false, seatNoop},
			{true, systemd.ForegroundUnknown, on, true, false, seatNoop},
		}},
		{"disabled never acts", []tick{
			{false, systemd.ForegroundOther, on, true, false, seatNoop},
			{false, systemd.ForegroundSelf, blk, true, false, seatNoop},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &seatWatchState{}
			for i, tk := range c.ticks {
				probe := seatProbe{
					nginx:      func() string { return tk.nginx },
					owed:       func() bool { return tk.owed },
					otherHolds: func() bool { return tk.otherHolds },
				}
				if got := s.decide(tk.enabled, tk.fg, probe); got != tk.want {
					t.Fatalf("tick %d: decide = %v, want %v", i, got, tk.want)
				}
			}
		})
	}
}
