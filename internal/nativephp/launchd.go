package nativephp

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// buildFPMPlist renders the launchd job that supervises a native PHP-FPM.
// KeepAlive restarts it if it dies; RunAtLoad starts it on login, matching how
// the containerised FPM comes back after a reboot. Env is sorted so a
// regeneration produces a byte-identical file and never restarts FPM for free.
func buildFPMPlist(label string, args []string, env map[string]string, logPath string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>`)
	b.WriteString(esc(label))
	b.WriteString("</string>\n\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, a := range args {
		b.WriteString("\t\t<string>" + esc(a) + "</string>\n")
	}
	b.WriteString("\t</array>\n")
	if len(env) > 0 {
		b.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("\t\t<key>" + esc(k) + "</key>\n")
			b.WriteString("\t\t<string>" + esc(env[k]) + "</string>\n")
		}
		b.WriteString("\t</dict>\n")
	}
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n\t<true/>\n")
	b.WriteString("\t<key>StandardOutPath</key>\n\t<string>" + esc(logPath) + "</string>\n")
	b.WriteString("\t<key>StandardErrorPath</key>\n\t<string>" + esc(logPath) + "</string>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// UnitLabel is the launchd label supervising a version's native FPM.
func UnitLabel(version string) string {
	return "lerd-native-php" + strings.ReplaceAll(version, ".", "")
}

// ConfPath is the generated php-fpm.conf for a version's native listener.
func ConfPath(version string) string {
	return filepath.Join(config.RunDir(), "native", version, "php-fpm.conf")
}

// LogPath is where a version's native FPM writes.
func LogPath(version string) string {
	return filepath.Join(logDir(), UnitLabel(version)+".log")
}

// logDir matches where every other lerd launchd job writes, so `lerd logs` and
// the TUI tail find a native FPM the same way they find a container one.
func logDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "lerd")
}

func plistPath(version string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", UnitLabel(version)+".plist")
}

// Ensure brings a version's native FPM up: config and overrides on disk, the
// launchd job installed, and the process running. Safe to call repeatedly; an
// unchanged plist is left alone so a running FPM is not restarted for nothing.
func Ensure(version string) error {
	binary := FPMBinaryPath(version)
	if err := EnsureInstalled(version, binary); err != nil {
		return err
	}
	if err := WriteOverrides(version); err != nil {
		return err
	}
	conf := ConfPath(version)
	config.GuardRealWrite(conf)
	if err := os.MkdirAll(filepath.Dir(conf), 0755); err != nil {
		return err
	}
	body, err := FPMConfig(version, LogPath(version))
	if err != nil {
		return err
	}
	if err := os.WriteFile(conf, []byte(body), 0644); err != nil {
		return err
	}
	if err := os.MkdirAll(logDir(), 0755); err != nil {
		return err
	}

	label := UnitLabel(version)
	plist := buildFPMPlist(label,
		[]string{binary, "-y", conf},
		map[string]string{"PHP_INI_SCAN_DIR": IniScanDir(version)},
		LogPath(version))
	path := plistPath(version)
	config.GuardRealWrite(path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	existing, _ := os.ReadFile(path)
	if string(existing) != plist {
		if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
			return err
		}
		// The job is running the old definition, so it has to be replaced
		// rather than left alone by the already-loaded check below.
		_ = exec.Command("launchctl", "bootout", domainTarget(label)).Run()
	}
	return bootLaunchdUnit(label, path)
}

// Stop takes a version's native FPM down and removes its launchd job.
func Stop(version string) error {
	label := UnitLabel(version)
	path := plistPath(version)
	_ = exec.Command("launchctl", "bootout", domainTarget(label)).Run()
	config.GuardRealWrite(path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func domainTarget(label string) string {
	return fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
}

// bootLaunchdUnit loads the job if it is not loaded already. Ensure runs on
// every start, not only on a runtime switch, so bootstrapping unconditionally
// reported "Bootstrap failed: 5" once per PHP version on a healthy install.
func bootLaunchdUnit(label, path string) error {
	return bootLaunchdUnitWith(label, path, unitLoaded, bootstrapUnit)
}

// bootLaunchdUnitWith is bootLaunchdUnit with its launchctl calls injected, so
// the skip-when-loaded decision is testable without touching the real domain.
func bootLaunchdUnitWith(label, path string, loaded func(string) bool, bootstrap func(string, string) error) error {
	if loaded(label) {
		return nil
	}
	return bootstrap(label, path)
}

// unitLoaded reports whether launchd already has the job in the user domain.
func unitLoaded(label string) bool {
	return exec.Command("launchctl", "print", domainTarget(label)).Run() == nil
}

func bootstrapUnit(label, path string) error {
	out, err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap %s: %v: %s", label, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Reload restarts a version's native FPM so a changed php.ini reaches the
// processes handling requests. Ensure is deliberately a no-op for a listener
// that is already up, which is right on start and wrong after an ini edit.
func Reload(version string) error {
	if err := Ensure(version); err != nil {
		return err
	}
	return reloadWith(UnitLabel(version), unitLoaded, restartUnit)
}

// reloadWith is Reload's decision with its launchctl calls injected. A listener
// that is not loaded was just started by Ensure and needs no kick.
func reloadWith(label string, loaded func(string) bool, restart func(string) error) error {
	if !loaded(label) {
		return nil
	}
	return restart(label)
}

func restartUnit(label string) error {
	out, err := exec.Command("launchctl", "kickstart", "-k", domainTarget(label)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl kickstart -k %s: %v: %s", label, err, strings.TrimSpace(string(out)))
	}
	return nil
}
