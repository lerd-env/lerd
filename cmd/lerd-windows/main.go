// Command lerd-windows builds lerd.exe: `lerd <args>` runs lerd in the distro
// from the current folder, php.exe and the other shim names run that shim, and
// `lerd --agent` boots the distro at login and answers DNS for the site TLD.
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
	winshim.DriveShare = driveShare
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

// agent returns once its config is removed or rewritten, which is how uninstall
// and a rerun of wsl:setup stop it: an elevated agent cannot be killed unelevated.
func agent(cfg winshim.Config, confPath string) error {
	started, err := os.Stat(confPath)
	if err != nil {
		return err
	}
	// Starting any process boots the distro, and with it systemd and lerd's
	// services; instanceIdleTimeout=-1 keeps it up after this one exits.
	boot := func() {
		args := []string{"-d", cfg.Distro}
		if cfg.User != "" {
			args = append(args, "-u", cfg.User)
		}
		if out, err := exec.Command("wsl.exe", append(args, "--exec", "/bin/true")...).CombinedOutput(); err != nil {
			log.Printf("booting %s: %v: %s", cfg.Distro, err, out)
		}
	}
	go boot()
	go mirrorShims(cfg, filepath.Dir(confPath))
	// Another program can hold port 53 for a while; the agent keeps trying
	// rather than staying alive and silent, which the desktop app would not
	// restart since the process has not exited.
	go func() {
		for {
			err := serveDNS(cfg, boot)
			if err == nil {
				return
			}
			log.Printf("%v; retrying in 10s", err)
			time.Sleep(10 * time.Second)
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

// lerdUp reports lerd-ui answering, which means the distro is running.
func lerdUp() bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:7073", 500*time.Millisecond)
	if err == nil {
		c.Close()
	}
	return err == nil
}

// mirrorShims keeps a .exe twin beside lerd.exe for each of lerd's shims, so a
// client tool a service adds later, psql say, runs from Windows too. It reads
// them only while lerd is up, so it never boots the distro itself.
func mirrorShims(cfg winshim.Config, dir string) {
	if cfg.Shims == "" {
		return
	}
	src := winshim.UNCPath(cfg.Distro, cfg.Shims)
	for {
		if lerdUp() {
			names, err := winshim.ShimNames(src)
			if err == nil {
				err = winshim.SyncTwins(dir, names)
			}
			if err != nil {
				log.Printf("mirroring shims: %v", err)
			}
		}
		time.Sleep(30 * time.Second)
	}
}

// serveDNS answers *.<tld> on 127.0.0.1:53, where the NRPT rule sends it, and
// boots the distro again when a site is looked up while lerd is down.
func serveDNS(cfg winshim.Config, boot func()) error {
	waker := &winshim.Waker{
		Alive: lerdUp,
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
	defer pc.Close()
	log.Printf("answering *.%s on 127.0.0.1:53", cfg.TLD)
	go selfTest(cfg.TLD)
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

// selfTest asks the agent's own socket a real question, since one that bound
// fine can still receive nothing. It retries, as Windows may be busy at login.
func selfTest(tld string) {
	var err error
	for range 3 {
		if err = winshim.Probe("127.0.0.1:53", tld, 2*time.Second); err == nil {
			log.Print(winshim.ProbePassed)
			return
		}
	}
	log.Printf("%s: %v. Something on this machine is intercepting DNS, usually a DNS filter such as NextDNS, so *.%s will not resolve in Windows", winshim.ProbeFailed, err, tld)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "lerd:", err)
	os.Exit(1)
}
