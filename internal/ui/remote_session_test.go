package ui

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestRemoteSession_signValidatesRoundTrip(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	now := time.Unix(1_700_000_000, 0)
	val := remoteSessionSign("alice", string(hash), now.Add(remoteSessionTTL))

	if !remoteSessionValid(val, "alice", string(hash), now) {
		t.Error("freshly signed cookie failed to validate")
	}
}

func TestRemoteSession_rejectsTamperedAndStale(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	h := string(hash)
	now := time.Unix(1_700_000_000, 0)
	val := remoteSessionSign("alice", h, now.Add(remoteSessionTTL))

	otherHash, _ := bcrypt.GenerateFromPassword([]byte("different"), bcrypt.MinCost)

	// Flip the final MAC character to a definitely-different hex digit.
	flip := byte('0')
	if val[len(val)-1] == '0' {
		flip = '1'
	}
	tampered := val[:len(val)-1] + string(flip)

	cases := []struct {
		name string
		val  string
		user string
		hash string
		now  time.Time
		want bool
	}{
		{"valid", val, "alice", h, now, true},
		{"wrong user", val, "bob", h, now, false},
		{"password changed", val, "alice", string(otherHash), now, false},
		{"expired", val, "alice", h, now.Add(remoteSessionTTL + time.Minute), false},
		{"tampered mac", tampered, "alice", h, now, false},
		{"empty hash", val, "alice", "", now, false},
		{"malformed", "not-a-cookie", "alice", h, now, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := remoteSessionValid(c.val, c.user, c.hash, c.now); got != c.want {
				t.Errorf("remoteSessionValid = %v, want %v", got, c.want)
			}
		})
	}
}
