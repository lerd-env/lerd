package cli

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/hostbin"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/tools"
	"github.com/geodro/lerd/internal/webserver"
)

// Seams for Homebrew, so a test never installs or upgrades anything.
var (
	hostNginxLookFn = hostbin.Look
	brewInstallFn   = func(brew, formula string, w io.Writer) error {
		cmd := exec.Command(brew, "install", formula)
		cmd.Stdout, cmd.Stderr = w, w
		return cmd.Run()
	}
	brewUpgradeFn = func(brew, formula string) error {
		out, err := exec.Command(brew, "upgrade", formula).CombinedOutput()
		if err != nil {
			return fmt.Errorf("brew upgrade %s: %v: %s", formula, err, out)
		}
		return nil
	}
)

// ensureHostNginx installs Homebrew's nginx before PHP moves to the host, which
// takes nginx with it. Without Homebrew, or kept in its container on request,
// nginx stays where it is and the switch says why.
func ensureHostNginx(w io.Writer) {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg.Nginx.Runtime == config.NginxRuntimeContainer {
		return
	}
	if _, ok := hostNginxLookFn("nginx"); ok {
		return
	}
	brew, ok := hostNginxLookFn("brew")
	if !ok {
		feedback.Warn("nginx stays in its container: Homebrew is not installed to provide one for the host")
		return
	}
	fmt.Fprintln(w, "Installing nginx with Homebrew, to serve beside native PHP")
	if err := brewInstallFn(brew, "nginx", w); err != nil {
		feedback.Warn("installing nginx: %v (nginx stays in its container)", err)
	}
}

// upgradeHostNginx moves Homebrew's nginx to the version its index carries and
// restarts lerd's nginx on it, the update the Tools list offers for it.
func upgradeHostNginx() error {
	brew, ok := hostNginxLookFn("brew")
	if !ok {
		return fmt.Errorf("Homebrew is not installed")
	}
	if err := brewUpgradeFn(brew, "nginx"); err != nil {
		return err
	}
	if webserver.OnHost() {
		if err := podman.RestartUnit(podman.WebServerUnit); err != nil {
			return fmt.Errorf("restarting nginx on the new version: %w", err)
		}
	}
	// The Tools list caches what it read; this is no stamp, so say so here.
	if tools.OnStampWritten != nil {
		tools.OnStampWritten()
	}
	return nil
}
