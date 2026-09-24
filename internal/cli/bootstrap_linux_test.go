//go:build linux

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/dns"
)

// stubBootstrapSystem redirects the three root actions runBootstrapSystem
// performs so the test never touches /etc or the real login manager.
func stubBootstrapSystem(t *testing.T) (*[][]string, *[]string) {
	t.Helper()
	origPath, origRunner, origSudoers, origOwns := unprivPortDropIn, bootstrapRunner, writeDNSSudoers, dns.HostOwnsResolver
	origInitramfs := initramfsHasPortDropIn
	t.Cleanup(func() {
		unprivPortDropIn, bootstrapRunner, writeDNSSudoers = origPath, origRunner, origSudoers
		dns.HostOwnsResolver = origOwns
		initramfsHasPortDropIn = origInitramfs
	})
	initramfsHasPortDropIn = func() bool { return false }
	unprivPortDropIn = filepath.Join(t.TempDir(), "99-lerd-ports.conf")
	dns.HostOwnsResolver = func() bool { return false }

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

// NixOS owns systemd-resolved from configuration.nix. Writing the DNS sudoers
// grant there lets a later start or watcher apply lerd0 and empty FallbackDNS
// without a prompt, which takes down all name resolution.
func TestRunBootstrapSystemSkipsSudoersOnNixOS(t *testing.T) {
	runs, users := stubBootstrapSystem(t)
	dns.HostOwnsResolver = func() bool { return true }

	if err := runBootstrapSystem("george", false); err != nil {
		t.Fatalf("runBootstrapSystem: %v", err)
	}
	if len(*users) != 0 {
		t.Errorf("sudoers grant written on NixOS: %v", *users)
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

func TestRemovePortDropInDeletesItThroughSudo(t *testing.T) {
	runs, _ := stubBootstrapSystem(t)
	if err := os.WriteFile(unprivPortDropIn, []byte(unprivPortSetting+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	removePortDropIn()

	want := []string{"sudo", "rm", "-f", unprivPortDropIn}
	if len(*runs) != 1 || strings.Join((*runs)[0], " ") != strings.Join(want, " ") {
		t.Errorf("commands = %v, want [%v]", *runs, want)
	}
}

// No drop-in means lerd never lowered the port start, so there is nothing to
// ask sudo for.
func TestRemovePortDropInSkipsWhenAbsent(t *testing.T) {
	runs, _ := stubBootstrapSystem(t)

	removePortDropIn()

	if len(*runs) != 0 {
		t.Errorf("commands = %v, want none", *runs)
	}
}

// dracut copies /etc/sysctl.d into the initramfs, which goes on applying the
// drop-in at boot until it is rebuilt, so the user gets the rebuild command.
func TestRemovePortDropInWarnsWhenInitramfsKeepsIt(t *testing.T) {
	stubBootstrapSystem(t)
	stubInitramfsHasPortDropIn(t, true)
	if err := os.WriteFile(unprivPortDropIn, []byte(unprivPortSetting+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, removePortDropIn)

	if !strings.Contains(out, "sudo dracut -f") {
		t.Errorf("output = %q, want the dracut rebuild command", out)
	}
}

func TestRemovePortDropInQuietWhenInitramfsIsClean(t *testing.T) {
	stubBootstrapSystem(t)
	stubInitramfsHasPortDropIn(t, false)
	if err := os.WriteFile(unprivPortDropIn, []byte(unprivPortSetting+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, removePortDropIn)

	if strings.Contains(out, "dracut") {
		t.Errorf("output = %q, want no rebuild hint", out)
	}
}

func stubInitramfsHasPortDropIn(t *testing.T, has bool) {
	t.Helper()
	initramfsHasPortDropIn = func() bool { return has }
}
