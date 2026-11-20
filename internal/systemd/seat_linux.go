//go:build linux

package systemd

import (
	"context"
	"os"
	"time"
)

// SeatForeground reports whose session has seat0's screen. lerd is rootless
// per user, so on a shared machine only the user on screen can hold its ports.
func SeatForeground() Foreground {
	c, err := login1Connect()
	if err != nil {
		return ForegroundUnknown
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	list, err := c.ListSessionsContext(ctx)
	if err != nil {
		return ForegroundUnknown
	}
	var sessions []seatSession
	for _, s := range list {
		if s.Seat != "seat0" {
			continue
		}
		props, err := c.GetSessionPropertiesContext(ctx, s.Path)
		if err != nil {
			return ForegroundUnknown
		}
		active, _ := props["Active"].Value().(bool)
		class, _ := props["Class"].Value().(string)
		sessions = append(sessions, seatSession{UID: s.UID, Active: active, Class: class})
	}
	return seatForeground(sessions, uint32(os.Getuid()))
}

type seatSession struct {
	UID    uint32
	Active bool
	Class  string
}

func seatForeground(sessions []seatSession, selfUID uint32) Foreground {
	if len(sessions) == 0 {
		return ForegroundUnknown
	}
	for _, s := range sessions {
		if !s.Active {
			continue
		}
		switch {
		case s.UID == selfUID:
			return ForegroundSelf
		case s.Class == "user":
			return ForegroundOther
		}
	}
	// The greeter is on screen, or nothing is: keep whichever stack is up.
	return ForegroundNobody
}
