//go:build linux

package cli

import (
	"path/filepath"
	"testing"
)

// stubBootstrapSystem redirects the three root actions runBootstrapSystem
// performs so the test never touches /etc or the real login manager.
func stubBootstrapSystem(t *testing.T) (*[][]string, *[]string) {
	t.Helper()
	origPath, origRunner, origSudoers := unprivPortDropIn, bootstrapRunner, writeDNSSudoers
	t.Cleanup(func() {
		unprivPortDropIn, bootstrapRunner, writeDNSSudoers = origPath, origRunner, origSudoers
	})
	unprivPortDropIn = filepath.Join(t.TempDir(), "99-lerd-ports.conf")

	var runs [][]string
	bootstrapRunner = func(name string, args ...string) error {
		runs = append(runs, append([]string{name}, args...))
		return nil
	}
	var sudoersUsers []string
	writeDNSSudoers = func(user string) error {
		sudoersUsers = append(sudoersUsers, user)
		return nil
	}
	return &runs, &sudoersUsers
}

func TestRunBootstrapSystemWritesSudoers(t *testing.T) {
	runs, users := stubBootstrapSystem(t)

	if err := runBootstrapSystem("george", false); err != nil {
		t.Fatalf("runBootstrapSystem: %v", err)
	}
	if len(*runs) != 2 || (*runs)[0][0] != "sysctl" || (*runs)[1][0] != "loginctl" {
		t.Errorf("commands = %v, want sysctl then loginctl", *runs)
	}
	if len(*users) != 1 || (*users)[0] != "george" {
		t.Errorf("sudoers written for %v, want [george]", *users)
	}
}

// A localhost-mode install manages no DNS, so it must not be left holding a
// standing passwordless resolver grant it never uses.
func TestRunBootstrapSystemSkipsSudoers(t *testing.T) {
	runs, users := stubBootstrapSystem(t)

	if err := runBootstrapSystem("george", true); err != nil {
		t.Fatalf("runBootstrapSystem: %v", err)
	}
	if len(*users) != 0 {
		t.Errorf("sudoers grant written despite --skip-sudoers: %v", *users)
	}
	if len(*runs) != 2 {
		t.Errorf("commands = %v, want ports and linger still applied", *runs)
	}
}

func TestRunBootstrapSystemNoTargetUser(t *testing.T) {
	runs, users := stubBootstrapSystem(t)

	if err := runBootstrapSystem("", false); err != nil {
		t.Fatalf("runBootstrapSystem: %v", err)
	}
	// Ports are machine-global and still apply; linger and sudoers are per-user
	// and have nobody to apply to.
	if len(*runs) != 1 || (*runs)[0][0] != "sysctl" {
		t.Errorf("commands = %v, want sysctl only", *runs)
	}
	if len(*users) != 0 {
		t.Errorf("sudoers written with no target user: %v", *users)
	}
}
