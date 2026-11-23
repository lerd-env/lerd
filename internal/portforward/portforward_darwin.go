//go:build darwin

package portforward

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/pfredirect"
)

// Vars so tests can stand in for root.
var (
	runSudo      = sudo
	removeLegacy = pfredirect.Remove
)

// WriteFunc writes a root-owned file, prompting for a password if needed.
type WriteFunc func(path string, content []byte, mode os.FileMode) error

// ErrNotEffective means launchd took a job but loopback connections do not
// reach nginx's port through it.
var ErrNotEffective = errors.New("launchd holds the port but loopback connections do not reach nginx through it; check that nothing else listens on 127.0.0.1:80 or 443, then run lerd install")

// Pending reports whether Install has root work to do, which needs a password.
func Pending(user string) bool {
	if _, err := os.Stat(pfredirect.AnchorPath); err == nil {
		return true
	}
	for _, p := range pfredirect.Ports() {
		if !current(p, user) {
			return true
		}
	}
	return false
}

func current(port int, user string) bool {
	got, err := os.ReadFile(PlistPath(port))
	return err == nil && string(got) == Render(port, user, config.LerdBinary())
}

// Install replaces the legacy pf rule with a launchd job per privileged port,
// and takes the jobs back out when loopback does not reach nginx through them.
// moved reports whether nginx's publish changed, so the caller restarts it.
func Install(user string, writeFile WriteFunc) (moved bool, err error) {
	if user == "" {
		return false, errors.New("cannot determine the current user")
	}
	had := len(Redirected()) > 0
	defer func() { moved = had != (len(Redirected()) > 0) }()
	// The writes go first: they are what asks for the password, so a failed
	// prompt leaves the old redirect working rather than nothing.
	ports := pfredirect.Ports()
	var changed []int
	for _, p := range ports {
		if current(p, user) {
			continue
		}
		if err := writeFile(PlistPath(p), []byte(Render(p, user, config.LerdBinary())), 0644); err != nil {
			return false, fmt.Errorf("writing %s: %w", PlistPath(p), err)
		}
		changed = append(changed, p)
	}
	if err := removeLegacy(); err != nil {
		return false, fmt.Errorf("removing the pf redirect: %w", err)
	}
	if err := removeJobs(func(p int) bool { return !slices.Contains(ports, p) }); err != nil {
		return false, err
	}
	for _, p := range changed {
		runSudo("launchctl", "bootout", "system/"+Label(p)) //nolint:errcheck // not loaded on a first install
		if err := runSudo("launchctl", "bootstrap", "system", PlistPath(p)); err != nil {
			return false, errors.Join(err, Remove())
		}
	}
	if !Effective() {
		return false, errors.Join(ErrNotEffective, Remove())
	}
	return false, nil
}

// Remove takes down every port-forward job and the legacy pf rule.
func Remove() error {
	return errors.Join(removeJobs(func(int) bool { return true }), removeLegacy())
}

// removeJobs boots out and deletes the installed jobs whose port drop picks.
func removeJobs(drop func(port int) bool) error {
	paths, _ := filepath.Glob(filepath.Join(PlistDir, "com.lerd.port-forward-*.plist"))
	for _, path := range paths {
		var port int
		if _, err := fmt.Sscanf(filepath.Base(path), "com.lerd.port-forward-%d.plist", &port); err != nil || !drop(port) {
			continue
		}
		runSudo("launchctl", "bootout", "system/"+Label(port)) //nolint:errcheck // may already be unloaded
		if err := runSudo("rm", "-f", path); err != nil {
			return err
		}
	}
	return nil
}

func sudo(args ...string) error {
	out, err := exec.Command("sudo", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sudo %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
