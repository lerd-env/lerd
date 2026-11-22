//go:build linux

package systemd

import "testing"

func TestSeatForeground(t *testing.T) {
	const self = 1001
	cases := []struct {
		name     string
		sessions []seatSession
		want     Foreground
	}{
		{"our desktop is on screen", []seatSession{{UID: 1000, Class: "user"}, {UID: self, Class: "user", Active: true}}, ForegroundSelf},
		{"another user's desktop is on screen", []seatSession{{UID: 1000, Class: "user", Active: true}, {UID: self, Class: "user"}}, ForegroundOther},
		{"the login screen is nobody's", []seatSession{{UID: 120, Class: "greeter", Active: true}, {UID: self, Class: "user"}}, ForegroundNobody},
		{"no active session on the seat", []seatSession{{UID: self, Class: "user"}}, ForegroundNobody},
		{"no seat at all, as over ssh or in WSL", nil, ForegroundUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := seatForeground(c.sessions, self); got != c.want {
				t.Errorf("seatForeground = %v, want %v", got, c.want)
			}
		})
	}
}
