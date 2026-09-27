// Command lerd-windows builds lerd.exe, the Windows side of a WSL install.
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

	"github.com/geodro/lerd/internal/winshim"
)

func main() {
	self, err := os.Executable()
	if err != nil {
		fail(err)
	}
	dir := filepath.Dir(self)
	b, err := os.ReadFile(filepath.Join(dir, winshim.ConfigName))
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
		log.Fatal(agent(cfg))
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

func agent(cfg winshim.Config) error {
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
		n, addr, err := pc.ReadFrom(buf)
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
