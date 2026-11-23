//go:build darwin

package pfredirect

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runSudo is a var so tests can stand in for root.
var runSudo = sudo

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
