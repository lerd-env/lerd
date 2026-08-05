package cli

import "testing"

// The reference documents `lerd unlink [name]`, and link, pause, secure and
// restart all take one. Without it a site whose directory moved or was deleted,
// or one of two registrations for the same directory, cannot be unlinked at all.
func TestUnlinkAcceptsASiteName(t *testing.T) {
	cmd := NewUnlinkCmd()
	if err := cmd.Args(cmd, []string{"mysite"}); err != nil {
		t.Errorf("unlink rejected a site name: %v", err)
	}
	if err := cmd.Args(cmd, []string{"a", "b"}); err == nil {
		t.Error("unlink accepted two names")
	}
	if err := cmd.Args(cmd, []string{}); err != nil {
		t.Errorf("unlink rejected the no-argument form: %v", err)
	}
}
