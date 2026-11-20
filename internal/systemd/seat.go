package systemd

// Foreground names whose session has the machine's screen (logind's seat0).
type Foreground int

const (
	// ForegroundUnknown is no seat or no readable logind: ssh-only, WSL, macOS.
	ForegroundUnknown Foreground = iota
	ForegroundSelf
	// ForegroundOther is another user's desktop session.
	ForegroundOther
	// ForegroundNobody is the login screen, or no active session on the seat.
	ForegroundNobody
)
