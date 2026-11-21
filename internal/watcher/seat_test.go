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
		procsUp    bool // lerd-ui or lerd-dns running
		otherHolds bool
		want       seatAction
	}
	cases := []struct {
		name  string
		ticks []tick
	}{
		{"switching away releases once, switching back resumes", []tick{
			{true, systemd.ForegroundSelf, on, true, false, seatNoop},
			{true, systemd.ForegroundOther, on, true, false, seatRelease},
			{true, systemd.ForegroundOther, off, false, false, seatNoop},
			{true, systemd.ForegroundSelf, off, false, false, seatResume},
			{true, systemd.ForegroundSelf, on, true, false, seatNoop},
		}},
		{"resume waits until the other user's lerd lets go of the ports", []tick{
			{true, systemd.ForegroundOther, on, true, false, seatRelease},
			{true, systemd.ForegroundSelf, off, false, true, seatNoop},
			{true, systemd.ForegroundSelf, off, false, false, seatResume},
		}},
		{"switching away again before the resume ran releases nothing new", []tick{
			{true, systemd.ForegroundOther, on, true, false, seatRelease},
			{true, systemd.ForegroundSelf, off, false, true, seatNoop},
			{true, systemd.ForegroundOther, off, false, false, seatNoop},
			{true, systemd.ForegroundSelf, off, false, false, seatResume},
		}},
		{"a lerd start while the resume waits is released on the next switch away", []tick{
			{true, systemd.ForegroundOther, on, true, false, seatRelease},
			{true, systemd.ForegroundSelf, off, false, true, seatNoop},
			{true, systemd.ForegroundOther, blk, true, false, seatRelease},
			{true, systemd.ForegroundSelf, off, false, false, seatResume},
		}},
		{"logging in after the other user's lerd blocked our start", []tick{
			{true, systemd.ForegroundSelf, blk, true, true, seatNoop},
			{true, systemd.ForegroundSelf, blk, true, false, seatResume},
			{true, systemd.ForegroundSelf, blk, true, false, seatNoop},
		}},
		{"a stack stopped by hand is released but never started", []tick{
			{true, systemd.ForegroundSelf, off, true, false, seatNoop},
			{true, systemd.ForegroundOther, off, true, false, seatRelease},
			{true, systemd.ForegroundSelf, off, false, false, seatNoop},
			{true, systemd.ForegroundOther, off, false, false, seatNoop},
			{true, systemd.ForegroundSelf, off, false, false, seatNoop},
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
					procsUp:    func() bool { return tk.procsUp },
					otherHolds: func() bool { return tk.otherHolds },
				}
				if got := s.decide(tk.enabled, tk.fg, probe); got != tk.want {
					t.Fatalf("tick %d: decide = %v, want %v", i, got, tk.want)
				}
			}
		})
	}
}
