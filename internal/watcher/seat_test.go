package watcher

import (
	"testing"

	"github.com/geodro/lerd/internal/systemd"
)

func TestSeatWatchState(t *testing.T) {
	type tick struct {
		enabled     bool
		fg          systemd.Foreground
		nginxFailed bool
		otherHolds  bool
		want        seatAction
	}
	cases := []struct {
		name  string
		ticks []tick
	}{
		{"switching away releases once, switching back resumes", []tick{
			{true, systemd.ForegroundSelf, false, false, seatNoop},
			{true, systemd.ForegroundOther, false, false, seatRelease},
			{true, systemd.ForegroundOther, false, false, seatNoop},
			{true, systemd.ForegroundSelf, false, false, seatResume},
			{true, systemd.ForegroundSelf, false, false, seatNoop},
		}},
		{"resume waits until the other user's lerd lets go of the ports", []tick{
			{true, systemd.ForegroundOther, false, false, seatRelease},
			{true, systemd.ForegroundSelf, false, true, seatNoop},
			{true, systemd.ForegroundSelf, false, false, seatResume},
		}},
		{"logging in after the other user's lerd blocked our start", []tick{
			{true, systemd.ForegroundSelf, true, true, seatNoop},
			{true, systemd.ForegroundSelf, true, false, seatResume},
			{true, systemd.ForegroundSelf, true, false, seatNoop},
		}},
		{"a stack stopped by hand stays stopped", []tick{
			{true, systemd.ForegroundSelf, false, false, seatNoop},
			{true, systemd.ForegroundSelf, false, false, seatNoop},
		}},
		{"the login screen and an unknown seat leave the stack alone", []tick{
			{true, systemd.ForegroundNobody, false, false, seatNoop},
			{true, systemd.ForegroundUnknown, false, false, seatNoop},
		}},
		{"disabled never acts", []tick{
			{false, systemd.ForegroundOther, false, false, seatNoop},
			{false, systemd.ForegroundSelf, true, false, seatNoop},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &seatWatchState{}
			for i, tk := range c.ticks {
				blocked := func() bool { return tk.nginxFailed }
				holds := func() bool { return tk.otherHolds }
				if got := s.decide(tk.enabled, tk.fg, blocked, holds); got != tk.want {
					t.Fatalf("tick %d: decide = %v, want %v", i, got, tk.want)
				}
			}
		})
	}
}
