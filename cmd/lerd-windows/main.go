// Command lerd-windows builds lerd.exe, the Windows side of a WSL install.
// Started with no config beside it, it is the installer (see setup.go).
// `lerd <args>` runs lerd inside the distro from the current folder. `lerd
// --agent`, started at login by wsl:setup, boots the distro so lerd's services
// come up without a terminal, then answers DNS for the site TLD on
// 127.0.0.1:53, where a Windows NRPT rule sends it.
package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/geodro/lerd/internal/winshim"
)

func main() {
	self, err := os.Executable()
	if err != nil {
		fail(err)
	}
	dir := filepath.Dir(self)
	b, err := os.ReadFile(filepath.Join(dir, winshim.ConfigName))
	// Without the config beside it this is the downloaded installer, not the
	// shim wsl:setup put on PATH.
	if (err != nil && len(os.Args) == 1) || (len(os.Args) > 1 && os.Args[1] == "--setup") {
		err := setup()
		if errors.Is(err, errHandedOff) {
			return
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "\nSetup stopped:", err)
		}
		pause()
		return
	}
	if err != nil {
		fail(fmt.Errorf("reading %s: %w (run `lerd wsl:setup` inside WSL)", winshim.ConfigName, err))
	}
	cfg, err := winshim.ParseConfig(string(b))
	if err != nil {
		fail(err)
	}
	if len(os.Args) > 1 && os.Args[1] == "--agent" {
		f, err := os.OpenFile(filepath.Join(dir, "lerd-agent.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err == nil {
			log.SetOutput(f)
		}
		log.Fatal(agent(cfg, filepath.Join(dir, winshim.ConfigName)))
	}
	os.Exit(shim(cfg))
}

func shim(cfg winshim.Config) int {
	cwd, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	cmd := exec.Command("wsl.exe", winshim.WSLArgs(cfg, cwd, os.Args[1:])...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		fail(err)
	}
	return 0
}

// agent keeps running until its config file is removed or rewritten, which is
// how uninstall and a rerun of wsl:setup stop it: the installer may have
// started it elevated, and an unelevated lerd is not allowed to kill that.
func agent(cfg winshim.Config, confPath string) error {
	started, err := os.Stat(confPath)
	if err != nil {
		return err
	}
	// Starting any process boots the distro, and with it systemd and lerd's
	// services; instanceIdleTimeout=-1 keeps it up after this one exits.
	if out, err := exec.Command("wsl.exe", "-d", cfg.Distro, "--exec", "/bin/true").CombinedOutput(); err != nil {
		log.Printf("booting %s: %v: %s", cfg.Distro, err, out)
	}
	if cfg.TLD == "" {
		log.Print("no tld configured, not answering DNS")
		return nil
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:53")
	if err != nil {
		return fmt.Errorf("listening on 127.0.0.1:53: %w", err)
	}
	log.Printf("answering *.%s on 127.0.0.1:53", cfg.TLD)
	buf := make([]byte, 1500)
	for {
		if now, err := os.Stat(confPath); err != nil || !now.ModTime().Equal(started.ModTime()) {
			log.Print("config removed or rewritten, stopping")
			return nil
		}
		pc.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
		n, addr, err := pc.ReadFrom(buf)
		if errors.Is(err, os.ErrDeadlineExceeded) {
			continue
		}
		if err != nil {
			return err
		}
		resp, err := winshim.Answer(buf[:n], cfg.TLD)
		if err != nil {
			continue // not a query we can parse; nothing sensible to reply
		}
		pc.WriteTo(resp, addr) //nolint:errcheck
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "lerd:", err)
	os.Exit(1)
}
