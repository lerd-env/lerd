package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// The machine-global, user-independent steps that `lerd install` would
// otherwise perform through interactive sudo. A package maintainer script runs
// as root but cannot prompt, so it calls `lerd bootstrap --system` for the
// prerequisites and `lerd bootstrap --trust-ca` after the per-user install, so
// `lerd install --unattended` in between needs no sudo. See lerd-env/lerd#979.
const unprivPortSetting = "net.ipv4.ip_unprivileged_port_start=80"

// Vars rather than consts so tests can redirect the root actions away from the
// real /etc and login manager.
var (
	unprivPortDropIn           = "/etc/sysctl.d/99-lerd-ports.conf"
	bootstrapRunner  cmdRunner = execRunner
)

// cmdRunner runs an external command. A seam so the bootstrap steps can be
// tested without touching the real system.
type cmdRunner func(name string, args ...string) error

func execRunner(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// writePortDropIn lets rootless Podman bind 80/443 by lowering
// ip_unprivileged_port_start, both live (sysctl -w) and persistently (a
// sysctl.d drop-in). Idempotent.
func writePortDropIn(path string, run cmdRunner) error {
	if err := os.WriteFile(path, []byte(unprivPortSetting+"\n"), 0644); err != nil {
		return err
	}
	return run("sysctl", "-w", unprivPortSetting)
}

// enableLinger keeps the user's rootless containers alive across screen blank,
// lock, and logout. A blank user is a no-op.
func enableLinger(user string, run cmdRunner) error {
	if user == "" {
		return nil
	}
	return run("loginctl", "enable-linger", user)
}

// bootstrapTargetUser resolves whose session the per-user half of setup belongs
// to. An explicit --user wins; otherwise SUDO_USER (the human who invoked the
// package manager) is preferred over the ambient USER, and a root SUDO_USER is
// ignored since it carries no per-user meaning.
func bootstrapTargetUser(flagUser string) string {
	if flagUser != "" {
		return flagUser
	}
	if u := os.Getenv("SUDO_USER"); u != "" && u != "root" {
		return u
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return os.Getenv("LOGNAME")
}

// NewBootstrapCmd returns the bootstrap command. It performs the root-level,
// non-interactive halves of setup so a deb/rpm postinst can finish the install
// without prompting, pairing with `lerd install --unattended`.
func NewBootstrapCmd() *cobra.Command {
	var system, trustCA, untrustCA, skipSudoers bool
	var user, caRoot string
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Apply the root-level system setup (for package maintainer scripts)",
		RunE: func(_ *cobra.Command, _ []string) error {
			if !system && !trustCA && !untrustCA {
				return fmt.Errorf("nothing to do: pass --system, --trust-ca or --untrust-ca")
			}
			if os.Geteuid() != 0 {
				return fmt.Errorf("lerd bootstrap configures system-level settings and must run as root")
			}
			switch {
			case trustCA:
				return runBootstrapTrustCA(bootstrapTargetUser(user), caRoot)
			case untrustCA:
				return runBootstrapUntrustCA()
			default:
				return runBootstrapSystem(bootstrapTargetUser(user), skipSudoers)
			}
		},
	}
	cmd.Flags().BoolVar(&system, "system", false, "Apply root-level setup: unprivileged ports, linger, DNS sudoers")
	cmd.Flags().BoolVar(&trustCA, "trust-ca", false, "Trust the user's mkcert CA in the system store (run after install)")
	cmd.Flags().BoolVar(&untrustCA, "untrust-ca", false, "Remove lerd's mkcert CA from the system store (run on uninstall)")
	cmd.Flags().BoolVar(&skipSudoers, "skip-sudoers", false,
		"With --system, leave out the passwordless DNS grant (for installs that manage no DNS)")
	cmd.Flags().StringVar(&user, "user", "", "Target user for per-user settings (defaults to SUDO_USER)")
	cmd.Flags().StringVar(&caRoot, "ca-root", "",
		"mkcert CAROOT holding the CA to trust (defaults to the target user's ~/.local/share/mkcert)")
	return cmd
}
