//go:build darwin

package pfredirect

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runSudo is a var so tests can stand in for root.
var runSudo = sudo

// WriteFunc writes a root-owned file, prompting for a password if needed.
type WriteFunc func(path string, content []byte, mode os.FileMode) error

// Pending reports whether Install has files to write, which needs a password.
func Pending() bool {
	_, err := os.Stat(SudoersPath)
	return err != nil || len(Redirected()) == 0
}

// Install writes the grant, then the anchor, and loads it. The grant goes
// first and a failed load takes the anchor back out: either way round, nginx
// would sit on a port nothing redirects to. moved reports whether nginx's
// publish changed, so the caller restarts it.
func Install(user string, writeFile WriteFunc) (moved bool, err error) {
	ports := Ports()
	if len(ports) == 0 {
		return false, nil
	}
	if user == "" {
		return false, errors.New("cannot determine the current user")
	}
	had := len(Redirected()) > 0
	defer func() { moved = had != (len(Redirected()) > 0) }()
	if _, statErr := os.Stat(SudoersPath); statErr != nil {
		if err := writeFile(SudoersPath, []byte(Sudoers(user)), 0440); err != nil {
			return false, fmt.Errorf("writing %s: %w", SudoersPath, err)
		}
	}
	if !had {
		if err := writeFile(AnchorPath, []byte(Render(ports)), 0644); err != nil {
			return false, fmt.Errorf("writing %s: %w", AnchorPath, err)
		}
	}
	if err := Apply(); err != nil {
		return false, errors.Join(err, runSudo("rm", "-f", AnchorPath))
	}
	return false, nil
}

// Apply loads the installed anchor without prompting, then checks loopback
// traffic takes it. pf drops anchor rules on reboot and on any
// `pfctl -f /etc/pf.conf`, so start re-applies it.
func Apply() error {
	if len(Redirected()) == 0 {
		return nil
	}
	if err := runSudo(append([]string{"-n"}, ApplyArgs()...)...); err != nil {
		return err
	}
	if !Effective() {
		return ErrNotEffective
	}
	return nil
}

// Remove flushes the anchor and deletes its file and grant. pf itself stays enabled,
// since other tools may hold a reference to it.
func Remove() error {
	if _, err := os.Stat(AnchorPath); err != nil {
		return nil
	}
	if err := runSudo(pfctl, "-a", Anchor, "-F", "all"); err != nil {
		return err
	}
	return runSudo("rm", "-f", AnchorPath, SudoersPath)
}

func sudo(args ...string) error {
	out, err := exec.Command("sudo", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sudo %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
