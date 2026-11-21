package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/systemd"
)

func TestSeatRefusesStart(t *testing.T) {
	cases := []struct {
		name      string
		multiUser bool
		fg        systemd.Foreground
		refuse    bool
	}{
		{"another user's desktop is on screen", true, systemd.ForegroundOther, true},
		{"our desktop is on screen", true, systemd.ForegroundSelf, false},
		{"the login screen is up", true, systemd.ForegroundNobody, false},
		{"no seat, as over ssh on a headless box", true, systemd.ForegroundUnknown, false},
		{"multi-user mode off", false, systemd.ForegroundOther, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := seatRefusesStart(c.multiUser, c.fg) != nil; got != c.refuse {
				t.Errorf("seatRefusesStart refused = %v, want %v", got, c.refuse)
			}
		})
	}
}
