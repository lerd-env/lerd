// Command lerd-windows builds lerd.exe, the Windows side of a WSL install.
// Started with no config beside it, it is the installer (see setup.go).
// `lerd <args>` runs lerd inside the distro from the current folder, and the
// same binary named php.exe, composer.exe and so on runs that shim. `lerd
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
	shim := winshim.ShimFromExe(self)
	// Without the config beside it this is the downloaded installer, not the
	// shim wsl:setup put on PATH.
	if shim == "" && ((err != nil && len(os.Args) == 1) || (len(os.Args) > 1 && os.Args[1] == "--setup")) {
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
	cwd, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	if shim != "" {
		args, err := winshim.ShimArgs(cfg, cwd, shim, os.Args[1:])
		if err != nil {
			fail(err)
		}
		os.Exit(runInDistro(args))
	}
	if len(os.Args) > 1 && os.Args[1] == "--agent" {
		// Before the log, which a second agent would otherwise truncate.
		if !claimAgent() {
			os.Exit(agentRunningExit)
		}
		f, err := os.OpenFile(filepath.Join(dir, "lerd-agent.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err == nil {
			log.SetOutput(f)
			// The headless console drops stderr, where a panic writes.
			redirectStderr(f)
		}
		if err := agent(cfg, filepath.Join(dir, winshim.ConfigName)); err != nil {
			log.Fatal(err)
		}
		return
	}
	os.Exit(runInDistro(winshim.WSLArgs(cfg, cwd, os.Args[1:])))
}

func runInDistro(wslArgs []string) int {
	cmd := exec.Command("wsl.exe", wslArgs...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		fail(err)
	}
	return 0
}

// agentRunningExit is the exit code of an agent that found another one
// running, so whoever started it can tell that from a failure.
const agentRunningExit = 3

// agent boots the distro and answers DNS for the site TLD. It returns once its config file is removed or
// rewritten, which is how uninstall and a rerun of wsl:setup stop it: the
// installer may have started it elevated, and an unelevated lerd is not
// allowed to kill that.
func agent(cfg winshim.Config, confPath string) error {
	started, err := os.Stat(confPath)
	if err != nil {
		return err
	}
	// Starting any process boots the distro, and with it systemd and lerd's
	// services; instanceIdleTimeout=-1 keeps it up after this one exits.
	boot := func() {
		if out, err := exec.Command("wsl.exe", "-d", cfg.Distro, "--exec", "/bin/true").CombinedOutput(); err != nil {
			log.Printf("booting %s: %v: %s", cfg.Distro, err, out)
		}
	}
	go boot()
	go func() {
		if err := serveDNS(cfg, boot); err != nil {
			log.Print(err)
		}
	}()
	for {
		time.Sleep(2 * time.Second)
		if now, err := os.Stat(confPath); err != nil || !now.ModTime().Equal(started.ModTime()) {
			log.Print("config removed or rewritten, stopping")
			return nil
		}
	}
}

// serveDNS answers *.<tld> on 127.0.0.1:53, where the NRPT rule sends it, and
// boots the distro again when a site is looked up while lerd is down.
func serveDNS(cfg winshim.Config, boot func()) error {
	waker := &winshim.Waker{
		Alive: func() bool {
			c, err := net.DialTimeout("tcp", "127.0.0.1:7073", 500*time.Millisecond)
			if err == nil {
				c.Close()
			}
			return err == nil
		},
		Boot: func() {
			log.Printf("a site was looked up while lerd was down, booting %s", cfg.Distro)
			boot()
		},
		Now:      time.Now,
		Interval: 30 * time.Second,
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
		go waker.Check()
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "lerd:", err)
	os.Exit(1)
}
